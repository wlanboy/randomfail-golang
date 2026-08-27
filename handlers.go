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
	mux.HandleFunc("GET /{$}", handleRoot(s))
	mux.HandleFunc("GET /status", handleStatus(s, cfg))

	mux.HandleFunc("POST /chaos/reset", handleChaosReset(s))
	mux.HandleFunc("POST /chaos/oom", handleChaosOOM(s, cfg))
	mux.HandleFunc("POST /chaos/cpu", handleChaosCPU(s, cfg))
	mux.HandleFunc("POST /chaos/crash", handleChaosCrash())
	mux.HandleFunc("POST /chaos/unhealthy", handleChaosUnhealthy(s))
	mux.HandleFunc("POST /chaos/slow", handleChaosSlow(s))
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

func handleRoot(s *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := s.nextRootRequestCount()
		if n%3 == 0 {
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
		s.reset()
		writeJSON(w, map[string]any{"status": "reset", "scenario": ScenarioStable})
	}
}

func handleChaosOOM(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const immediateBytes = 100_000_000 // 100 MB, per readme.md
		chunk := make([]byte, immediateBytes)
		for i := range chunk {
			chunk[i] = 1
		}
		s.appendMemChunk(chunk)
		s.startOOMGrowth(cfg.MemoryChunkSize)
		s.setScenario(ScenarioOOMKill)
		writeJSON(w, map[string]any{"status": "ok", "scenario": ScenarioOOMKill})
	}
}

func handleChaosCPU(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.startCPUBurn(cfg.CPUBurnThreads, cfg.CPUBurnDuration)
		s.setScenario(ScenarioCPUBurn)
		writeJSON(w, map[string]any{"status": "ok", "scenario": ScenarioCPUBurn})
	}
}

func handleChaosCrash() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"status": "crashing", "scenario": ScenarioCrash})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		go func() {
			time.Sleep(50 * time.Millisecond)
			log.Println("CRASH scenario triggered: exiting immediately")
			os.Exit(1)
		}()
	}
}

func handleChaosUnhealthy(s *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		healthy := s.toggleHealthy()
		if !healthy {
			s.setScenario(ScenarioSlowDeath)
		} else {
			s.setScenario(ScenarioStable)
		}
		writeJSON(w, map[string]any{"status": "ok", "healthy": healthy, "scenario": s.getScenario()})
	}
}

func handleChaosSlow(s *State) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.setSlowResponse(true)
		s.setScenario(ScenarioSlowResponse)
		writeJSON(w, map[string]any{"status": "ok", "scenario": ScenarioSlowResponse})
	}
}

func handleChaosFlap(s *State, cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.startFlap(cfg.ReadinessFlapInterval)
		s.setScenario(ScenarioReadinessFlap)
		writeJSON(w, map[string]any{"status": "ok", "scenario": ScenarioReadinessFlap})
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
