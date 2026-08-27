package main

import (
	"testing"
	"time"
)

func TestEnvIntDefault(t *testing.T) {
	if v := envInt("RF_TEST_UNSET_INT", 42); v != 42 {
		t.Errorf("envInt() with unset var = %d, want 42", v)
	}
}

func TestEnvIntParsed(t *testing.T) {
	t.Setenv("RF_TEST_INT", "7")
	if v := envInt("RF_TEST_INT", 42); v != 7 {
		t.Errorf("envInt() = %d, want 7", v)
	}
}

func TestEnvIntInvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("RF_TEST_INT", "not-a-number")
	if v := envInt("RF_TEST_INT", 42); v != 42 {
		t.Errorf("envInt() with invalid value = %d, want default 42", v)
	}
}

func TestEnvSecondsDefault(t *testing.T) {
	if v := envSeconds("RF_TEST_UNSET_SECONDS", 5); v != 5*time.Second {
		t.Errorf("envSeconds() with unset var = %s, want 5s", v)
	}
}

func TestEnvSecondsParsed(t *testing.T) {
	t.Setenv("RF_TEST_SECONDS", "3")
	if v := envSeconds("RF_TEST_SECONDS", 5); v != 3*time.Second {
		t.Errorf("envSeconds() = %s, want 3s", v)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	for _, name := range []string{
		"CHAOS_INTERVAL", "CHAOS_STARTUP_DELAY", "MEMORY_CHUNK_SIZE",
		"CPU_BURN_THREADS", "CPU_BURN_DURATION", "SLOW_RESPONSE_DELAY",
		"SIGTERM_DELAY", "READINESS_FLAP_INTERVAL",
	} {
		t.Setenv(name, "")
	}

	cfg := loadConfig()

	want := Config{
		ChaosInterval:         300 * time.Second,
		ChaosStartupDelay:     10 * time.Second,
		MemoryChunkSize:       1_000_000,
		CPUBurnThreads:        2,
		CPUBurnDuration:       120 * time.Second,
		SlowResponseDelay:     5 * time.Second,
		SigtermDelay:          30 * time.Second,
		ReadinessFlapInterval: 5 * time.Second,
	}
	if cfg != want {
		t.Errorf("loadConfig() = %+v, want %+v", cfg, want)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("CHAOS_INTERVAL", "60")
	t.Setenv("CHAOS_STARTUP_DELAY", "1")
	t.Setenv("MEMORY_CHUNK_SIZE", "2048")
	t.Setenv("CPU_BURN_THREADS", "4")
	t.Setenv("CPU_BURN_DURATION", "30")
	t.Setenv("SLOW_RESPONSE_DELAY", "2")
	t.Setenv("SIGTERM_DELAY", "15")
	t.Setenv("READINESS_FLAP_INTERVAL", "1")

	cfg := loadConfig()

	want := Config{
		ChaosInterval:         60 * time.Second,
		ChaosStartupDelay:     1 * time.Second,
		MemoryChunkSize:       2048,
		CPUBurnThreads:        4,
		CPUBurnDuration:       30 * time.Second,
		SlowResponseDelay:     2 * time.Second,
		SigtermDelay:          15 * time.Second,
		ReadinessFlapInterval: 1 * time.Second,
	}
	if cfg != want {
		t.Errorf("loadConfig() = %+v, want %+v", cfg, want)
	}
}
