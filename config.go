package main

import (
	"os"
	"strconv"
	"time"
)

// Config holds all tunables, sourced from environment variables (see readme.md).
type Config struct {
	ChaosInterval         time.Duration
	ChaosStartupDelay     time.Duration
	MemoryChunkSize       int
	CPUBurnThreads        int
	CPUBurnDuration       time.Duration
	SlowResponseDelay     time.Duration
	SigtermDelay          time.Duration
	ReadinessFlapInterval time.Duration
}

func loadConfig() Config {
	return Config{
		ChaosInterval:         envSeconds("CHAOS_INTERVAL", 300),
		ChaosStartupDelay:     envSeconds("CHAOS_STARTUP_DELAY", 10),
		MemoryChunkSize:       envInt("MEMORY_CHUNK_SIZE", 1_000_000),
		CPUBurnThreads:        envInt("CPU_BURN_THREADS", 2),
		CPUBurnDuration:       envSeconds("CPU_BURN_DURATION", 120),
		SlowResponseDelay:     envSeconds("SLOW_RESPONSE_DELAY", 5),
		SigtermDelay:          envSeconds("SIGTERM_DELAY", 30),
		ReadinessFlapInterval: envSeconds("READINESS_FLAP_INTERVAL", 5),
	}
}

func envInt(name string, def int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func envSeconds(name string, defSeconds int) time.Duration {
	return time.Duration(envInt(name, defSeconds)) * time.Second
}
