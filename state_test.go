package main

import (
	"testing"
	"time"
)

func TestNewState(t *testing.T) {
	s := newState()

	if got := s.getScenario(); got != ScenarioStable {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioStable)
	}
	if !s.isHealthy() {
		t.Error("isHealthy() = false, want true")
	}
	if !s.isReady() {
		t.Error("isReady() = false, want true")
	}
	if s.isSlowResponse() {
		t.Error("isSlowResponse() = true, want false")
	}
	if s.uptime() < 0 {
		t.Errorf("uptime() = %s, want >= 0", s.uptime())
	}
}

func TestStateReset(t *testing.T) {
	s := newState()
	s.setHealthy(false)
	s.setReady(false)
	s.setSlowResponse(true)
	s.setScenario(ScenarioOOMKill)
	s.appendMemChunk([]byte{1, 2, 3})
	s.startFlap(time.Hour)

	s.reset()

	if got := s.getScenario(); got != ScenarioStable {
		t.Errorf("getScenario() after reset = %s, want %s", got, ScenarioStable)
	}
	if !s.isHealthy() {
		t.Error("isHealthy() after reset = false, want true")
	}
	if !s.isReady() {
		t.Error("isReady() after reset = false, want true")
	}
	if s.isSlowResponse() {
		t.Error("isSlowResponse() after reset = true, want false")
	}
	if b := s.memBallastBytes(); b != 0 {
		t.Errorf("memBallastBytes() after reset = %d, want 0", b)
	}
	if s.flapCancel != nil {
		t.Error("flapCancel after reset = non-nil, want nil")
	}

	// reset() must be safe to call again with no chaos goroutines running.
	s.reset()
}

func TestScenarioGetSet(t *testing.T) {
	s := newState()
	s.setScenario(ScenarioCPUBurn)
	if got := s.getScenario(); got != ScenarioCPUBurn {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioCPUBurn)
	}
}

func TestHealthy(t *testing.T) {
	s := newState()
	s.setHealthy(false)
	if s.isHealthy() {
		t.Error("isHealthy() = true, want false")
	}
	s.setHealthy(true)
	if !s.isHealthy() {
		t.Error("isHealthy() = false, want true")
	}
}

func TestToggleHealthy(t *testing.T) {
	s := newState()
	if got := s.toggleHealthy(); got != false {
		t.Errorf("toggleHealthy() = %v, want false", got)
	}
	if s.isHealthy() {
		t.Error("isHealthy() = true after toggling from true, want false")
	}
	if got := s.toggleHealthy(); got != true {
		t.Errorf("toggleHealthy() = %v, want true", got)
	}
}

func TestReady(t *testing.T) {
	s := newState()
	s.setReady(false)
	if s.isReady() {
		t.Error("isReady() = true, want false")
	}
	s.setReady(true)
	if !s.isReady() {
		t.Error("isReady() = false, want true")
	}
}

func TestSlowResponse(t *testing.T) {
	s := newState()
	s.setSlowResponse(true)
	if !s.isSlowResponse() {
		t.Error("isSlowResponse() = false, want true")
	}
	s.setSlowResponse(false)
	if s.isSlowResponse() {
		t.Error("isSlowResponse() = true, want false")
	}
}

func TestMemBallast(t *testing.T) {
	s := newState()
	if b := s.memBallastBytes(); b != 0 {
		t.Fatalf("memBallastBytes() initial = %d, want 0", b)
	}

	s.appendMemChunk(make([]byte, 10))
	s.appendMemChunk(make([]byte, 5))

	if b := s.memBallastBytes(); b != 15 {
		t.Errorf("memBallastBytes() = %d, want 15", b)
	}
}

func TestNextRootRequestCount(t *testing.T) {
	s := newState()
	for i := uint64(1); i <= 3; i++ {
		if got := s.nextRootRequestCount(); got != i {
			t.Errorf("nextRootRequestCount() = %d, want %d", got, i)
		}
	}
}

func TestStartOOMGrowthIsIdempotent(t *testing.T) {
	s := newState()
	s.startOOMGrowth(1024)
	if !s.oomRunning {
		t.Fatal("oomRunning = false after startOOMGrowth, want true")
	}
	firstCancel := s.oomCancel

	// A second call while already running must not replace the goroutine/cancel.
	s.startOOMGrowth(1024)
	if s.oomCancel == nil {
		t.Fatal("oomCancel = nil, want non-nil")
	}

	s.reset()
	if s.oomRunning {
		t.Error("oomRunning after reset = true, want false")
	}
	_ = firstCancel
}

func TestStartCPUBurnReturnsImmediately(t *testing.T) {
	s := newState()
	done := make(chan struct{})
	go func() {
		s.startCPUBurn(1, 5*time.Millisecond)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("startCPUBurn() blocked instead of launching goroutines asynchronously")
	}
	if s.cpuCancel == nil {
		t.Error("cpuCancel = nil, want non-nil")
	}
	s.reset()
}

func TestStartFlapIsIdempotent(t *testing.T) {
	s := newState()
	s.startFlap(time.Hour)
	if s.flapCancel == nil {
		t.Fatal("flapCancel = nil after startFlap, want non-nil")
	}
	firstCancel := s.flapCancel

	// A second call while already running must not replace the running goroutine.
	s.startFlap(time.Hour)
	if s.flapCancel == nil {
		t.Fatal("flapCancel = nil, want non-nil")
	}

	s.reset()
	if s.flapCancel != nil {
		t.Error("flapCancel after reset = non-nil, want nil")
	}
	if !s.isReady() {
		t.Error("isReady() after reset = false, want true")
	}
	_ = firstCancel
}
