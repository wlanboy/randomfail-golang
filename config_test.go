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

func TestEnvBoolDefault(t *testing.T) {
	if v := envBool("RF_TEST_UNSET_BOOL", true); v != true {
		t.Errorf("envBool() with unset var = %v, want true", v)
	}
}

func TestEnvBoolParsed(t *testing.T) {
	t.Setenv("RF_TEST_BOOL", "false")
	if v := envBool("RF_TEST_BOOL", true); v != false {
		t.Errorf("envBool() = %v, want false", v)
	}
}

func TestEnvBoolInvalidFallsBackToDefault(t *testing.T) {
	t.Setenv("RF_TEST_BOOL", "not-a-bool")
	if v := envBool("RF_TEST_BOOL", true); v != true {
		t.Errorf("envBool() with invalid value = %v, want default true", v)
	}
}

func TestLoadConfigDefaults(t *testing.T) {
	for _, name := range []string{
		"CHAOS_ENABLED", "CHAOS_INTERVAL", "CHAOS_STARTUP_DELAY", "MEMORY_CHUNK_SIZE",
		"CPU_BURN_THREADS", "CPU_BURN_DURATION", "SLOW_RESPONSE_DELAY",
		"SIGTERM_DELAY", "READINESS_FLAP_INTERVAL",
	} {
		t.Setenv(name, "")
	}

	cfg := loadConfig()

	want := Config{
		ChaosEnabled:          true,
		ChaosInterval:         300 * time.Second,
		ChaosStartupDelay:     10 * time.Second,
		MemoryChunkSize:       1_000_000,
		CPUBurnThreads:        2,
		CPUBurnDuration:       120 * time.Second,
		SlowResponseDelay:     5 * time.Second,
		SigtermDelay:          30 * time.Second,
		ReadinessFlapInterval: 20 * time.Second,
	}
	if cfg != want {
		t.Errorf("loadConfig() = %+v, want %+v", cfg, want)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	t.Setenv("CHAOS_ENABLED", "false")
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
		ChaosEnabled:          false,
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

func TestEnvIntNegativeFallsBackToDefault(t *testing.T) {
	t.Setenv("RF_TEST_INT", "-1")
	if v := envInt("RF_TEST_INT", 42); v != 42 {
		t.Errorf("envInt() with negative value = %d, want default 42", v)
	}
}

func TestEnvIntZeroAllowed(t *testing.T) {
	t.Setenv("RF_TEST_INT", "0")
	if v := envInt("RF_TEST_INT", 42); v != 0 {
		t.Errorf("envInt() with 0 = %d, want 0", v)
	}
}

func TestEnvPositiveIntRejectsZeroAndNegative(t *testing.T) {
	for _, in := range []string{"0", "-5"} {
		t.Setenv("RF_TEST_INT", in)
		if v := envPositiveInt("RF_TEST_INT", 42); v != 42 {
			t.Errorf("envPositiveInt(%q) = %d, want default 42", in, v)
		}
	}
}

func TestEnvSecondsUnitSuffixFallsBackToDefault(t *testing.T) {
	t.Setenv("RF_TEST_SECONDS", "5m")
	if v := envSeconds("RF_TEST_SECONDS", 300); v != 300*time.Second {
		t.Errorf("envSeconds(\"5m\") = %s, want default 300s", v)
	}
}

func TestLoadConfigRejectsNonPositiveIntervals(t *testing.T) {
	t.Setenv("CHAOS_INTERVAL", "0")
	t.Setenv("READINESS_FLAP_INTERVAL", "-1")
	t.Setenv("CPU_BURN_THREADS", "0")
	t.Setenv("MEMORY_CHUNK_SIZE", "0")
	t.Setenv("CPU_BURN_DURATION", "0")
	t.Setenv("SIGTERM_DELAY", "0")

	cfg := loadConfig()

	if cfg.ChaosInterval != 300*time.Second {
		t.Errorf("ChaosInterval = %s, want default 300s", cfg.ChaosInterval)
	}
	if cfg.ReadinessFlapInterval != 20*time.Second {
		t.Errorf("ReadinessFlapInterval = %s, want default 20s", cfg.ReadinessFlapInterval)
	}
	if cfg.CPUBurnThreads != 2 {
		t.Errorf("CPUBurnThreads = %d, want default 2", cfg.CPUBurnThreads)
	}
	if cfg.MemoryChunkSize != 1_000_000 {
		t.Errorf("MemoryChunkSize = %d, want default 1000000", cfg.MemoryChunkSize)
	}
	if cfg.CPUBurnDuration != 120*time.Second {
		t.Errorf("CPUBurnDuration = %s, want default 120s", cfg.CPUBurnDuration)
	}
	if cfg.SigtermDelay != 0 {
		t.Errorf("SigtermDelay = %s, want 0 (zero delay is allowed)", cfg.SigtermDelay)
	}
}
