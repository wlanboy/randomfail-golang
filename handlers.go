package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"time"
)

func newMux(s *State, cfg Config) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", handleHealthz(s))
	mux.HandleFunc("GET /readyz", handleReadyz(s))
	mux.HandleFunc("GET /{$}", handleRoot(s, cfg))
	mux.HandleFunc("GET /status", handleStatus(s, cfg))

	mux.HandleFunc("POST /chaos/reset", handleChaosReset(s))
	mux.HandleFunc("POST /chaos/oom", handleChaosOOM(s, cfg))
	mux.HandleFunc("POST /chaos/cpu", handleChaosCPU(s, cfg))
	mux.HandleFunc("POST /chaos/crash", handleChaosCrash(s, cfg))
	mux.HandleFunc("POST /chaos/unhealthy", handleChaosUnhealthy(s, cfg))
	mux.HandleFunc("POST /chaos/slow", handleChaosSlow(s, cfg))
	mux.HandleFunc("POST /chaos/flap", handleChaosFlap(s, cfg))

	return slowResponseMiddleware(s, cfg, mux)
}

// slowResponseMiddleware delays every request (including probes) when the
// SLOW_RESPONSE scenario is active, per readme.md.
func slowResponseMiddleware(s *State, cfg Config, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.isSlowResponse() {
			time.Sleep(cfg.SlowResponseDelay)
		}
		next.ServeHTTP(w, r)
	})
}

func handleHealthz(s *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isHealthy() {
			http.Error(w, "unhealthy", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	}
}

func handleReadyz(s *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.isReady() {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}
}

// handleRoot serves the HTML status page. With CHAOS_ENABLED every third
// request fails with HTTP 500; with chaos disabled it always succeeds.
func handleRoot(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := s.nextRootRequestCount()
		if cfg.ChaosEnabled && n%3 == 0 {
			http.Error(w, "simulated error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!doctype html>
<html>
<head><title>randomfail</title></head>
<body>
<h1>randomfail</h1>
<p>Zeit: %s</p>
<p>Aktives Szenario: %s</p>
</body>
</html>`, time.Now().Format(time.RFC3339), s.getScenario())
	}
}

func handleStatus(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var mem runtime.MemStats
		runtime.ReadMemStats(&mem)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"scenario":      s.getScenario(),
			"healthy":       s.isHealthy(),
			"ready":         s.isReady(),
			"uptimeSeconds": int(s.uptime().Seconds()),
			"memory": map[string]any{
				"allocBytes":   mem.Alloc,
				"sysBytes":     mem.Sys,
				"ballastBytes": s.memBallastBytes(),
			},
			"fileDescriptors": countOpenFDs(),
			"goroutines":      runtime.NumGoroutine(),
			"config": map[string]any{
				"chaosEnabled":                 cfg.ChaosEnabled,
				"chaosIntervalSeconds":         int(cfg.ChaosInterval.Seconds()),
				"chaosStartupDelaySeconds":     int(cfg.ChaosStartupDelay.Seconds()),
				"memoryChunkSize":              cfg.MemoryChunkSize,
				"cpuBurnThreads":               cfg.CPUBurnThreads,
				"cpuBurnDurationSeconds":       int(cfg.CPUBurnDuration.Seconds()),
				"slowResponseDelaySeconds":     int(cfg.SlowResponseDelay.Seconds()),
				"sigtermDelaySeconds":          int(cfg.SigtermDelay.Seconds()),
				"readinessFlapIntervalSeconds": int(cfg.ReadinessFlapInterval.Seconds()),
			},
		})
	}
}

func countOpenFDs() int {
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return -1
	}
	return len(entries)
}

func handleChaosReset(s *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resetScenario(s)
		writeJSON(w, map[string]any{"status": "reset", "scenario": s.getScenario()})
	}
}

// writeChaosResult answers a /chaos/* request: 200 with the now active
// scenario, or 409 if the switch was refused because of a SIGTERM shutdown.
func writeChaosResult(w http.ResponseWriter, s *State, ok bool) {
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"status": "shutting down", "scenario": s.getScenario()})
		return
	}
	writeJSON(w, map[string]any{"status": "ok", "healthy": s.isHealthy(), "scenario": s.getScenario()})
}

func handleChaosOOM(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.isShuttingDown() {
			writeChaosResult(w, s, false)
			return
		}
		const immediateBytes = 100_000_000 // 100 MB, per readme.md
		chunk := make([]byte, immediateBytes)
		for i := range chunk {
			chunk[i] = 1
		}
		// Hold activateMu across activation and the extra ballast so a
		// concurrent switch can't reset in between and leave 100 MB behind.
		s.activateMu.Lock()
		ok := activateScenarioLocked(s, cfg, ScenarioOOMKill)
		if ok {
			s.appendMemChunk(chunk)
		}
		s.activateMu.Unlock()
		writeChaosResult(w, s, ok)
	}
}

func handleChaosCPU(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeChaosResult(w, s, activateScenario(s, cfg, ScenarioCPUBurn))
	}
}

func handleChaosCrash(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.isShuttingDown() {
			writeChaosResult(w, s, false)
			return
		}
		writeJSON(w, map[string]any{"status": "crashing", "scenario": ScenarioCrash})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		go func() {
			time.Sleep(50 * time.Millisecond)
			// Re-checked here: SIGTERM may have arrived after the response.
			if !activateScenario(s, cfg, ScenarioCrash) {
				log.Println("CRASH refused: SIGTERM shutdown in progress")
			}
		}()
	}
}

// handleChaosUnhealthy toggles between SLOW_DEATH and a clean STABLE state.
func handleChaosUnhealthy(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next := ScenarioSlowDeath
		if s.getScenario() == ScenarioSlowDeath {
			next = ScenarioStable
		}
		writeChaosResult(w, s, activateScenario(s, cfg, next))
	}
}

func handleChaosSlow(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeChaosResult(w, s, activateScenario(s, cfg, ScenarioSlowResponse))
	}
}

func handleChaosFlap(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeChaosResult(w, s, activateScenario(s, cfg, ScenarioReadinessFlap))
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
