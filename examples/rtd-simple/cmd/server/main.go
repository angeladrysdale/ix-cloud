// SPDX-License-Identifier: Apache-2.0

// Command rtd-simple is a no-op Index Cloud gRPC RTD service for exercising the
// onboarding process. It implements the ARTF RTBExtensionPoint GetMutations RPC
// (returning zero mutations), the gRPC health service, and the required HTTP
// metrics/health endpoints. Every request is logged so operators can watch what
// Index sends into the container during onboarding.
//
// Ports/paths come from the environment Index sets at start ("Building your
// container → Environment variables"):
//
//	GRPC_PORT   gRPC RTBExtensionPoint + gRPC health service (default 50051)
//	SERVER_PORT HTTP /metrics, /heartbeat (liveness), /healthcheck (readiness)
//	MODEL_PATH  directory model data is mounted into (logged, not required here)
//
// Run with -healthcheck to self-probe /healthcheck and exit 0/1 (Docker
// HEALTHCHECK; the final image has no shell).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/indexexchange/rtd-simple/internal/agent"
	"github.com/indexexchange/rtd-simple/internal/config"
	"github.com/indexexchange/rtd-simple/internal/health"
	"github.com/indexexchange/rtd-simple/internal/metrics"
	"github.com/indexexchange/rtd-simple/internal/model"
	"github.com/indexexchange/rtd-simple/internal/runtimetune"
	pb "github.com/indexexchange/rtd-simple/pkg/pb/artf"

	"google.golang.org/grpc"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

var version = "dev"

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe the local /healthcheck endpoint and exit 0/1")
	flag.Parse()

	cfg := config.Load()
	if cfg.APIVersion == "0.1.0" && version != "dev" {
		cfg.APIVersion = version
	}
	if *healthcheck {
		os.Exit(selfProbe(cfg.ServerPort))
	}
	if err := run(cfg); err != nil {
		log.Fatalf("rtd-simple: %v", err)
	}
}

func run(cfg config.Config) error {
	log.Printf("rtd-simple %s starting: grpc=:%d http=:%d model_path=%s log_every=%d",
		cfg.APIVersion, cfg.GRPCPort, cfg.ServerPort, cfg.ModelPath, cfg.LogEvery)

	// Match the Go runtime to the cgroup limits (cuts GC overhead and avoids CPU
	// oversubscription/throttling). Env overrides win. See internal/runtimetune.
	log.Printf("runtime tuning: %s", runtimetune.Apply())

	reg := metrics.New(cfg.APIVersion)
	checker := health.New(cfg.APIVersion)
	httpSrv := startHTTP(cfg.ServerPort, reg, checker)

	// Check for a delivered model and report it (models are optional; this
	// container is a no-op and does not read model data).
	mi := model.Check(cfg.ModelPath)
	logModel(cfg.ModelPath, mi)
	reg.SetModel(mi.Present, mi.Label())

	// The agent logs asynchronously off the request path; agCtx stops the drainer
	// on shutdown so buffered lines flush.
	agCtx, agCancel := context.WithCancel(context.Background())
	defer agCancel() // stops the log drainer and flushes on any return path
	ag := agent.New(cfg.APIVersion, cfg.LogEvery)
	go ag.Run(agCtx)

	grpcSrv := grpc.NewServer(grpc.UnaryInterceptor(metricsInterceptor(reg)))
	pb.RegisterRTBExtensionPointServer(grpcSrv, ag)
	healthpb.RegisterHealthServer(grpcSrv, checker.GRPCServer())
	reflection.Register(grpcSrv)

	lis, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.GRPCPort))
	if err != nil {
		return fmt.Errorf("grpc listen: %w", err)
	}
	grpcErr := make(chan error, 1)
	go func() {
		log.Printf("gRPC RTBExtensionPoint listening on :%d", cfg.GRPCPort)
		grpcErr <- grpcSrv.Serve(lis)
	}()

	// No model to warm; ready immediately.
	checker.SetReady(true)
	reg.SetReady(true)
	log.Printf("rtd-simple ready (no-op: all responses return zero mutations)")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
	case err := <-grpcErr:
		if err != nil {
			return fmt.Errorf("grpc serve: %w", err)
		}
	}
	checker.SetReady(false)
	reg.SetReady(false)
	grpcSrv.GracefulStop()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	log.Printf("rtd-simple stopped")
	return nil
}

// logModel reports the model-presence check so onboarding can confirm the
// model-delivery step wired up. Models are optional; absence is not an error.
func logModel(path string, mi model.Info) {
	switch {
	case mi.Err != nil:
		log.Printf("model check at %s: %v (models are optional; continuing as no-op)", path, mi.Err)
	case mi.Dir == "":
		log.Printf("model check at %s: no model found (optional; container runs as a no-op)", path)
	case !mi.Present:
		log.Printf("model check at %s: manifest %q INCOMPLETE, missing %v (optional)", mi.Dir, mi.Name, mi.Missing)
	default:
		log.Printf("model present: %q files=%d %v (at %s)", mi.Name, len(mi.Files), mi.Files, mi.Dir)
	}
}

func startHTTP(port int, reg *metrics.Registry, checker *health.Checker) *http.Server {
	mux := http.NewServeMux()
	mux.Handle("/metrics", reg.Handler())
	mux.HandleFunc("/heartbeat", checker.LivenessHandler)
	mux.HandleFunc("/healthcheck", checker.ReadinessHandler)
	srv := &http.Server{Addr: ":" + strconv.Itoa(port), Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("HTTP metrics/health listening on :%d", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http serve: %v", err)
		}
	}()
	return srv
}

// metricsInterceptor records the required rpc_server_duration histogram for every
// RPC, tagged with the gRPC status code (numeric) and method.
func metricsInterceptor(reg *metrics.Registry) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		durSec := time.Since(start).Seconds() // metric is in seconds per the doc
		code := int(status.Code(err))
		errType := ""
		if err != nil {
			errType = status.Code(err).String()
		}
		service, method := splitMethod(info.FullMethod)
		reg.ObserveRPC(service, method, strconv.Itoa(code), errType, durSec)
		// Log errors only (per-request happy-path logging is opt-in in the agent),
		// so normal traffic does not flood the logs during validation or in prod.
		if err != nil {
			log.Printf("rtd-simple rpc error: method=%s code=%s: %v", method, status.Code(err), err)
		}
		return resp, err
	}
}

// splitMethod turns "/pkg.Service/Method" into ("Service", "Method").
func splitMethod(full string) (service, method string) {
	full = strings.TrimPrefix(full, "/")
	i := strings.LastIndex(full, "/")
	if i < 0 {
		return "", full
	}
	svc := full[:i]
	method = full[i+1:]
	if j := strings.LastIndex(svc, "."); j >= 0 {
		svc = svc[j+1:] // short service name, e.g. RTBExtensionPoint
	}
	return svc, method
}

func selfProbe(port int) int {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthcheck", port))
	if err != nil {
		fmt.Fprintln(os.Stderr, "healthcheck:", err)
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return 0
	}
	fmt.Fprintln(os.Stderr, "healthcheck: status", resp.StatusCode)
	return 1
}
