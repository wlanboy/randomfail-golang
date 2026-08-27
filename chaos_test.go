package main

import (
	"testing"
	"time"
)

// testCfg keeps every timing-related knob short so tests that trigger
// background goroutines (OOM growth, CPU burn, readiness flap) finish fast.
func testCfg() Config {
	return Config{
		ChaosInterval:         time.Millisecond,
		ChaosStartupDelay:     0,
		MemoryChunkSize:       16,
		CPUBurnThreads:        1,
		CPUBurnDuration:       time.Millisecond,
		SlowResponseDelay:     time.Millisecond,
		SigtermDelay:          time.Millisecond,
		ReadinessFlapInterval: time.Hour,
	}
}

// ScenarioCrash is intentionally not exercised here: activateScenario calls
// os.Exit(1) for it, which would kill the test process.
func TestActivateScenario(t *testing.T) {
	cfg := testCfg()

	tests := []struct {
		name          string
		scenario      Scenario
		wantHealthy   bool
		wantSlow      bool
		wantOOMRun    bool
		wantFlapSetUp bool
	}{
		{name: "stable", scenario: ScenarioStable, wantHealthy: true},
		{name: "oom kill", scenario: ScenarioOOMKill, wantHealthy: true, wantOOMRun: true},
		{name: "cpu burn", scenario: ScenarioCPUBurn, wantHealthy: true},
		{name: "slow death", scenario: ScenarioSlowDeath, wantHealthy: false},
		{name: "slow response", scenario: ScenarioSlowResponse, wantHealthy: true, wantSlow: true},
		{name: "readiness flap", scenario: ScenarioReadinessFlap, wantHealthy: true, wantFlapSetUp: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newState()
			defer s.reset()

			activateScenario(s, cfg, tt.scenario)

			if got := s.getScenario(); got != tt.scenario {
				t.Errorf("getScenario() = %s, want %s", got, tt.scenario)
			}
			if got := s.isHealthy(); got != tt.wantHealthy {
				t.Errorf("isHealthy() = %v, want %v", got, tt.wantHealthy)
			}
			if got := s.isSlowResponse(); got != tt.wantSlow {
				t.Errorf("isSlowResponse() = %v, want %v", got, tt.wantSlow)
			}
			if got := s.oomRunning; got != tt.wantOOMRun {
				t.Errorf("oomRunning = %v, want %v", got, tt.wantOOMRun)
			}
			if tt.wantFlapSetUp && s.flapCancel == nil {
				t.Error("flapCancel = nil, want non-nil")
			}
		})
	}
}

func TestActivateScenarioResetsPreviousState(t *testing.T) {
	cfg := testCfg()
	s := newState()
	defer s.reset()

	activateScenario(s, cfg, ScenarioSlowDeath)
	if s.isHealthy() {
		t.Fatal("isHealthy() = true after SLOW_DEATH, want false")
	}

	activateScenario(s, cfg, ScenarioStable)
	if !s.isHealthy() {
		t.Error("isHealthy() = false after activating STABLE, want true")
	}
	if got := s.getScenario(); got != ScenarioStable {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioStable)
	}
}

func TestAutoScenariosDoesNotRepeatEntries(t *testing.T) {
	seen := make(map[Scenario]bool)
	for _, sc := range autoScenarios {
		if seen[sc] {
			t.Errorf("autoScenarios contains duplicate entry %s", sc)
		}
		seen[sc] = true
	}
}
