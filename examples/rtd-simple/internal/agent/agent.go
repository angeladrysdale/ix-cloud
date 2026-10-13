// SPDX-License-Identifier: MIT-0

// Package agent implements a no-op RTBExtensionPoint used to exercise the Index
// Cloud onboarding process end to end.
//
// GetMutations inspects each RTBRequest, logs what it saw, and returns an empty
// (but valid) mutation set. It changes nothing about the auction, so it is safe
// to onboard against live traffic. The logging is the point: it shows operators
// exactly what Index sends into a container and confirms the request/response
// contract, the health signalling, and the required metric, with zero
// decisioning risk.
//
// Logging follows the "Building a Docker container" guidance: lines go to STDERR
// (Index collects STDOUT/STDERR), and emission is asynchronous and non-blocking
// off the request path ("Log and aggregate telemetry asynchronously"), so it
// never threatens the 5ms SLA. Request volume and latency live in the metrics
// endpoint (rpc_server_duration), per "prefer metrics over verbose logging"; the
// per-request log is an onboarding visibility aid, sampled via RTD_LOG_SAMPLE_N.
//
// NOTE: the shipped ARTF Go bindings ship stale Lifecycle enums (only
// *_UNSPECIFIED), so the lifecycle values are declared locally against the
// generated enum type using their proto field numbers.
package agent

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync/atomic"
	"time"

	pb "github.com/indexexchange/rtd-simple/pkg/pb/artf"
	openrtb "github.com/indexexchange/rtd-simple/pkg/pb/openrtb"
)

// Lifecycle values (proto field numbers) missing from the shipped bindings.
const (
	lifecyclePublisherBidRequest pb.Lifecycle = 1 // LIFECYCLE_PUBLISHER_BID_REQUEST
	lifecycleDSPBidResponse      pb.Lifecycle = 2 // LIFECYCLE_DSP_BID_RESPONSE
)

// logBuffer is how many formatted log lines can queue before new ones are
// dropped (and counted) rather than blocking the request path.
const logBuffer = 4096

// Agent implements pb.RTBExtensionPointServer as a logging no-op.
type Agent struct {
	pb.UnimplementedRTBExtensionPointServer
	apiVersion string
	logEvery   int64
	counter    atomic.Int64
	logCh      chan string
	dropped    atomic.Int64
}

// New builds a no-op Agent. logEvery <= 0 disables per-request logging (the
// default: errors only); logEvery = N logs 1 of every N requests, for onboarding
// or debugging. Call Run once (in a goroutine) to drain the log queue.
func New(apiVersion string, logEvery int) *Agent {
	if logEvery < 0 {
		logEvery = 0
	}
	return &Agent{apiVersion: apiVersion, logEvery: int64(logEvery), logCh: make(chan string, logBuffer)}
}

// Run drains queued log lines to STDERR asynchronously until ctx is cancelled,
// then flushes what remains and reports any drops. Start it with `go ag.Run(ctx)`.
func (a *Agent) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case msg := <-a.logCh:
					log.Print(msg)
				default:
					if d := a.dropped.Load(); d > 0 {
						log.Printf("rtd-simple: %d log lines dropped (buffer full)", d)
					}
					return
				}
			}
		case msg := <-a.logCh:
			log.Print(msg)
		}
	}
}

// GetMutations returns an empty mutation set. Per-request logging is off by
// default (errors are logged by the server interceptor); set RTD_LOG_SAMPLE_N > 0
// to also log 1-in-N requests for onboarding. A no-op has no inner work to bound,
// so tmax is not enforced.
func (a *Agent) GetMutations(_ context.Context, req *pb.RTBRequest) (*pb.RTBResponse, error) {
	start := time.Now()

	resp := &pb.RTBResponse{
		Id:        strPtr(req.GetId()), // echo the request id (required field)
		Mutations: nil,                 // no-op: an empty list is a valid response
		Metadata: &pb.Metadata{
			// The doc lists api_version and model_version as the metadata fields.
			// This container loads no model, so model_version is reported as "none".
			ApiVersion:   strPtr(a.apiVersion),
			ModelVersion: strPtr("none"),
		},
	}

	// Verbose per-request logging is opt-in (RTD_LOG_SAMPLE_N > 0) to avoid a log
	// firehose under load; guard the modulo so logEvery=0 never divides by zero.
	if a.logEvery > 0 {
		if n := a.counter.Add(1); n%a.logEvery == 0 {
			a.enqueueLog(formatRequest(req, time.Since(start)))
		}
	}
	return resp, nil
}

// enqueueLog hands a line to the async drainer without ever blocking the request
// path: if the buffer is full, drop and count instead.
func (a *Agent) enqueueLog(msg string) {
	select {
	case a.logCh <- msg:
	default:
		a.dropped.Add(1)
	}
}

// formatRequest builds one structured line describing what Index sent in and what
// the container returned (always zero mutations).
func formatRequest(req *pb.RTBRequest, dur time.Duration) string {
	br := req.GetBidRequest()
	return fmt.Sprintf("rtd-simple no-op: lifecycle=%s req_id=%s bid_request=%s supply=%s imps=%d eids=%d tmax_ms=%d has_bid_response=%t mutations=0 dur_ms=%.3f",
		lifecycleName(req.GetLifecycle()),
		req.GetId(),
		br.GetId(),
		orNone(supplyKey(br)),
		len(br.GetImp()),
		eidCount(br),
		req.GetTmax(),
		req.GetBidResponse() != nil,
		float64(dur.Microseconds())/1000.0,
	)
}

func lifecycleName(l pb.Lifecycle) string {
	switch l {
	case lifecyclePublisherBidRequest:
		return "PUBLISHER_BID_REQUEST"
	case lifecycleDSPBidResponse:
		return "DSP_BID_RESPONSE"
	default:
		return "UNSPECIFIED"
	}
}

// supplyKey reports the supply source for the log line: site domain, else app
// bundle or domain.
func supplyKey(br *openrtb.BidRequest) string {
	if br == nil {
		return ""
	}
	if s := br.GetSite(); s != nil && s.GetDomain() != "" {
		return "site:" + strings.ToLower(s.GetDomain())
	}
	if app := br.GetApp(); app != nil {
		if b := app.GetBundle(); b != "" {
			return "app:" + strings.ToLower(b)
		}
		if d := app.GetDomain(); d != "" {
			return "app:" + strings.ToLower(d)
		}
	}
	return ""
}

func eidCount(br *openrtb.BidRequest) int {
	if br == nil {
		return 0
	}
	if u := br.GetUser(); u != nil {
		return len(u.GetEids())
	}
	return 0
}

func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

func strPtr(s string) *string { return &s }
