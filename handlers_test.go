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
	h := handleRoot(s)

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
	h := handleChaosUnhealthy(s)

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
	h := handleChaosSlow(s)

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
