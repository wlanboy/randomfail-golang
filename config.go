package main

import (
	"log"
	"os"
	"strconv"
	"time"
)

// Config holds all tunables, sourced from environment variables (see readme.md).
type Config struct {
	ChaosEnabled          bool
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
		ChaosEnabled:          envBool("CHAOS_ENABLED", true),
		ChaosInterval:         envPositiveSeconds("CHAOS_INTERVAL", 300),
		ChaosStartupDelay:     envSeconds("CHAOS_STARTUP_DELAY", 10),
		MemoryChunkSize:       envPositiveInt("MEMORY_CHUNK_SIZE", 1_000_000),
		CPUBurnThreads:        envPositiveInt("CPU_BURN_THREADS", 2),
		CPUBurnDuration:       envPositiveSeconds("CPU_BURN_DURATION", 120),
		SlowResponseDelay:     envSeconds("SLOW_RESPONSE_DELAY", 5),
		SigtermDelay:          envSeconds("SIGTERM_DELAY", 30),
		ReadinessFlapInterval: envPositiveSeconds("READINESS_FLAP_INTERVAL", 20),
	}
}

// envIntMin reads an integer env var. Unparsable values and values below min
// are logged and replaced by def.
func envIntMin(name string, def, min int) int {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		log.Printf("config: %s=%q is not an integer, using default %d", name, v, def)
		return def
	}
	if n < min {
		log.Printf("config: %s=%d must be >= %d, using default %d", name, n, min, def)
		return def
	}
	return n
}

// envInt reads a non-negative integer env var.
func envInt(name string, def int) int {
	return envIntMin(name, def, 0)
}

// envPositiveInt reads an integer env var that must be > 0.
func envPositiveInt(name string, def int) int {
	return envIntMin(name, def, 1)
}

// envSeconds reads a non-negative number of seconds.
func envSeconds(name string, defSeconds int) time.Duration {
	return time.Duration(envInt(name, defSeconds)) * time.Second
}

// envPositiveSeconds reads a number of seconds that must be > 0. Used for
// intervals: time.NewTicker panics on <= 0 and a zero CHAOS_INTERVAL would
// spin the chaos cycle without pause.
func envPositiveSeconds(name string, defSeconds int) time.Duration {
	return time.Duration(envPositiveInt(name, defSeconds)) * time.Second
}

func envBool(name string, def bool) bool {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		log.Printf("config: %s=%q is not a boolean, using default %t", name, v, def)
		return def
	}
	return b
}
