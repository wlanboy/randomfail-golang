package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleHealthz(t *testing.T) {
	s := newState()
	h := handleHealthz(s)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("healthy: status = %d, want %d", rec.Code, http.StatusOK)
	}

	s.setHealthy(false)
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("unhealthy: status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestHandleReadyz(t *testing.T) {
	s := newState()
	h := handleReadyz(s)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Errorf("ready: status = %d, want %d", rec.Code, http.StatusOK)
	}

	s.setReady(false)
	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("not ready: status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestHandleRootFailsEveryThirdRequest(t *testing.T) {
	s := newState()
	s.setScenario(ScenarioCPUBurn)
	h := handleRoot(s, Config{ChaosEnabled: true})

	wantCodes := []int{http.StatusOK, http.StatusOK, http.StatusInternalServerError, http.StatusOK}
	for i, want := range wantCodes {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != want {
			t.Errorf("request %d: status = %d, want %d", i+1, rec.Code, want)
		}
		if want == http.StatusOK && !strings.Contains(rec.Body.String(), string(ScenarioCPUBurn)) {
			t.Errorf("request %d: body does not mention active scenario %s", i+1, ScenarioCPUBurn)
		}
	}
}

func TestHandleRootNeverFailsWithChaosDisabled(t *testing.T) {
	s := newState()
	h := handleRoot(s, Config{ChaosEnabled: false})

	for i := 0; i < 6; i++ {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != http.StatusOK {
			t.Errorf("request %d: status = %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}
}

func TestHandleStatus(t *testing.T) {
	s := newState()
	s.appendMemChunk(make([]byte, 100))
	cfg := testCfg()
	h := handleStatus(s, cfg)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodGet, "/status", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON body: %v", err)
	}
	if got := body["scenario"]; got != string(ScenarioStable) {
		t.Errorf("scenario = %v, want %s", got, ScenarioStable)
	}
	if got := body["healthy"]; got != true {
		t.Errorf("healthy = %v, want true", got)
	}
	mem, ok := body["memory"].(map[string]any)
	if !ok {
		t.Fatalf("memory field missing or wrong type: %v", body["memory"])
	}
	if got := mem["ballastBytes"]; got != float64(100) {
		t.Errorf("ballastBytes = %v, want 100", got)
	}
	conf, ok := body["config"].(map[string]any)
	if !ok {
		t.Fatalf("config field missing or wrong type: %v", body["config"])
	}
	if got := conf["chaosEnabled"]; got != true {
		t.Errorf("config.chaosEnabled = %v, want true", got)
	}
}

func TestHandleChaosReset(t *testing.T) {
	s := newState()
	s.setHealthy(false)
	s.setScenario(ScenarioOOMKill)
	h := handleChaosReset(s)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/chaos/reset", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !s.isHealthy() {
		t.Error("isHealthy() after reset = false, want true")
	}
	if got := s.getScenario(); got != ScenarioStable {
		t.Errorf("getScenario() after reset = %s, want %s", got, ScenarioStable)
	}
}

func TestHandleChaosOOM(t *testing.T) {
	s := newState()
	defer s.reset()
	cfg := testCfg()
	h := handleChaosOOM(s, cfg)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/chaos/oom", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := s.getScenario(); got != ScenarioOOMKill {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioOOMKill)
	}
	if b := s.memBallastBytes(); b < 100_000_000 {
		t.Errorf("memBallastBytes() = %d, want >= 100000000", b)
	}
}

func TestHandleChaosCPU(t *testing.T) {
	s := newState()
	defer s.reset()
	cfg := testCfg()
	h := handleChaosCPU(s, cfg)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/chaos/cpu", nil))

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got := s.getScenario(); got != ScenarioCPUBurn {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioCPUBurn)
	}
}

func TestHandleChaosUnhealthy(t *testing.T) {
	s := newState()
	h := handleChaosUnhealthy(s, testCfg())

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/chaos/unhealthy", nil))
	if s.isHealthy() {
		t.Error("isHealthy() after first toggle = true, want false")
	}
	if got := s.getScenario(); got != ScenarioSlowDeath {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioSlowDeath)
	}

	rec = httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/chaos/unhealthy", nil))
	if !s.isHealthy() {
		t.Error("isHealthy() after second toggle = false, want true")
	}
	if got := s.getScenario(); got != ScenarioStable {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioStable)
	}
}

func TestHandleChaosSlow(t *testing.T) {
	s := newState()
	h := handleChaosSlow(s, testCfg())

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/chaos/slow", nil))

	if !s.isSlowResponse() {
		t.Error("isSlowResponse() = false, want true")
	}
	if got := s.getScenario(); got != ScenarioSlowResponse {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioSlowResponse)
	}
}

func TestHandleChaosFlap(t *testing.T) {
	s := newState()
	defer s.reset()
	cfg := testCfg()
	h := handleChaosFlap(s, cfg)

	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, "/chaos/flap", nil))

	if got := s.getScenario(); got != ScenarioReadinessFlap {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioReadinessFlap)
	}
	if s.flapCancel == nil {
		t.Error("flapCancel = nil, want non-nil")
	}
}

func TestSlowResponseMiddleware(t *testing.T) {
	s := newState()
	cfg := testCfg()
	cfg.SlowResponseDelay = 20 * time.Millisecond

	var called bool
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	mw := slowResponseMiddleware(s, cfg, next)

	start := time.Now()
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	elapsed := time.Since(start)

	if !called {
		t.Error("next handler was not called when SLOW_RESPONSE inactive")
	}
	if elapsed >= cfg.SlowResponseDelay {
		t.Errorf("request took %s without SLOW_RESPONSE active, want < %s", elapsed, cfg.SlowResponseDelay)
	}

	s.setSlowResponse(true)
	called = false
	start = time.Now()
	rec = httptest.NewRecorder()
	mw.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	elapsed = time.Since(start)

	if !called {
		t.Error("next handler was not called when SLOW_RESPONSE active")
	}
	if elapsed < cfg.SlowResponseDelay {
		t.Errorf("request took %s with SLOW_RESPONSE active, want >= %s", elapsed, cfg.SlowResponseDelay)
	}
}

// stubExit replaces exitFunc for the test and returns a channel receiving
// every exit code.
func stubExit(t *testing.T) chan int {
	t.Helper()
	codes := make(chan int, 4)
	orig := exitFunc
	exitFunc = func(code int) { codes <- code }
	t.Cleanup(func() { exitFunc = orig })
	return codes
}

func post(h http.HandlerFunc, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h(rec, httptest.NewRequest(http.MethodPost, path, nil))
	return rec
}

func TestManualScenariosDoNotStack(t *testing.T) {
	s := newState()
	defer s.reset()
	cfg := testCfg()

	post(handleChaosSlow(s, cfg), "/chaos/slow")
	post(handleChaosFlap(s, cfg), "/chaos/flap")

	if got := s.getScenario(); got != ScenarioReadinessFlap {
		t.Errorf("getScenario() = %s, want %s", got, ScenarioReadinessFlap)
	}
	if s.isSlowResponse() {
		t.Error("isSlowResponse() = true after /chaos/flap, want previous SLOW_RESPONSE cleared")
	}
}

func TestUnhealthyToggleStopsOOMGrowth(t *testing.T) {
	s := newState()
	defer s.reset()
	cfg := testCfg()

	post(handleChaosOOM(s, cfg), "/chaos/oom")
	post(handleChaosUnhealthy(s, cfg), "/chaos/unhealthy")
	if got := s.getScenario(); got != ScenarioSlowDeath {
		t.Fatalf("getScenario() = %s, want %s", got, ScenarioSlowDeath)
	}
	if s.oomRunning || s.memBallastBytes() != 0 {
		t.Error("OOM growth still active after switching to SLOW_DEATH")
	}

	post(handleChaosUnhealthy(s, cfg), "/chaos/unhealthy")
	if got := s.getScenario(); got != ScenarioStable || !s.isHealthy() {
		t.Errorf("after second toggle: scenario=%s healthy=%v, want STABLE/true", got, s.isHealthy())
	}
}

func TestChaosEndpointsRejectedDuringShutdown(t *testing.T) {
	codes := stubExit(t)
	s := newState()
	defer s.reset()
	cfg := testCfg()
	s.beginShutdown()

	handlers := map[string]http.HandlerFunc{
		"/chaos/oom":       handleChaosOOM(s, cfg),
		"/chaos/cpu":       handleChaosCPU(s, cfg),
		"/chaos/crash":     handleChaosCrash(s, cfg),
		"/chaos/unhealthy": handleChaosUnhealthy(s, cfg),
		"/chaos/slow":      handleChaosSlow(s, cfg),
		"/chaos/flap":      handleChaosFlap(s, cfg),
	}
	for path, h := range handlers {
		if rec := post(h, path); rec.Code != http.StatusConflict {
			t.Errorf("%s during shutdown: status = %d, want %d", path, rec.Code, http.StatusConflict)
		}
	}
	if got := s.getScenario(); got != ScenarioStable {
		t.Errorf("getScenario() = %s, want STABLE (nothing activated)", got)
	}

	select {
	case code := <-codes:
		t.Errorf("exit(%d) called during shutdown, want no exit", code)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestChaosCrashRefusedIfSigtermArrivesAfterResponse(t *testing.T) {
	codes := stubExit(t)
	s := newState()
	defer s.reset()

	rec := post(handleChaosCrash(s, testCfg()), "/chaos/crash")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	s.beginShutdown() // inside the 50 ms window before the exit

	select {
	case code := <-codes:
		t.Errorf("exit(%d) called after SIGTERM, want graceful shutdown untouched", code)
	case <-time.After(150 * time.Millisecond):
	}
}

func TestChaosCrashExitsWithCode1(t *testing.T) {
	codes := stubExit(t)
	s := newState()
	defer s.reset()

	post(handleChaosCrash(s, testCfg()), "/chaos/crash")

	select {
	case code := <-codes:
		if code != 1 {
			t.Errorf("exit code = %d, want 1", code)
		}
	case <-time.After(time.Second):
		t.Fatal("exit not called after /chaos/crash")
	}
}

func TestChaosResetAllowedDuringShutdown(t *testing.T) {
	s := newState()
	cfg := testCfg()
	activateScenario(s, cfg, ScenarioCPUBurn)
	s.beginShutdown()

	if rec := post(handleChaosReset(s), "/chaos/reset"); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if s.cpuCancel != nil {
		t.Error("CPU burn still running after reset")
	}
	if s.isHealthy() || s.isReady() {
		t.Error("reset revived probes during shutdown")
	}
}
