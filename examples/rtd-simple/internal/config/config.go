// SPDX-License-Identifier: MIT-0

// Package config reads the runtime environment variables Index Cloud sets at
// container start, per "Building your container → Environment variables", plus
// the one knob specific to this onboarding test container.
package config

import (
	"os"
	"strconv"
)

// Config holds the resolved runtime configuration.
type Config struct {
	// GRPCPort — port the gRPC RTBExtensionPoint service (and gRPC health) listen
	// on. Set by Index via GRPC_PORT; documented default 50051.
	GRPCPort int
	// ServerPort — port for the HTTP /metrics, /heartbeat, /healthcheck endpoints.
	// Set by Index via SERVER_PORT (no documented default; 8080 for local runs).
	ServerPort int
	// ModelPath — directory model data files are mounted into (MODEL_PATH). This
	// container loads no model; it only logs what is mounted there so onboarding
	// can confirm the model-delivery step wired up.
	ModelPath string
	// APIVersion — this container's version, reported in response metadata.
	APIVersion string

	// LogEvery — verbose per-request logging. 0 (default) logs errors only; N > 0
	// logs 1 of every N GetMutations calls, for onboarding or debugging. Kept off
	// by default so it cannot flood logs under load.
	LogEvery int
}

// Load resolves configuration from the environment with local-dev defaults so
// the container also runs standalone.
func Load() Config {
	return Config{
		GRPCPort:   getInt("GRPC_PORT", 50051),
		ServerPort: getInt("SERVER_PORT", 8080),
		ModelPath:  getStr("MODEL_PATH", "/models"),
		APIVersion: getStr("APP_VERSION", "0.1.0"),
		LogEvery:   getInt("RTD_LOG_SAMPLE_N", 0),
	}
}

func getStr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func getInt(key string, def int) int {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
