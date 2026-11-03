package main

import (
	"log"
	"math/rand"
	"time"
)

// autoScenarios are the failure states the automatic chaos cycle picks
// between. SIGTERM_DELAY is excluded: it is not a "current state" but the
// service's always-on shutdown behavior (see handleSigterm).
var autoScenarios = []Scenario{
	ScenarioStable,
	ScenarioOOMKill,
	ScenarioCPUBurn,
	ScenarioSlowDeath,
	ScenarioCrash,
	ScenarioSlowResponse,
	ScenarioReadinessFlap,
}

// runChaosCycle waits for the configured startup delay, then repeatedly
// resets state and activates a randomly chosen scenario every
// CHAOS_INTERVAL seconds. If CHAOS_ENABLED is false, it returns immediately
// and never runs the automatic cycle; manual /chaos/* endpoints still work.
func runChaosCycle(s *State, cfg Config) {
	if !cfg.ChaosEnabled {
		log.Println("chaos cycle: CHAOS_ENABLED=false, automatic chaos disabled")
		return
	}

	time.Sleep(cfg.ChaosStartupDelay)

	for {
		sc := autoScenarios[rand.Intn(len(autoScenarios))]
		activateScenario(s, cfg, sc)
		time.Sleep(cfg.ChaosInterval)
	}
}

// activateScenario switches to exactly one scenario: it resets all running
// chaos first, so scenarios never stack. It is the single entry point for
// both the chaos cycle and the /chaos/* endpoints. It returns false (and
// changes nothing) while a SIGTERM shutdown is in progress, so neither can
// revive the probes nor turn a graceful exit into a CRASH.
func activateScenario(s *State, cfg Config, sc Scenario) bool {
	s.activateMu.Lock()
	defer s.activateMu.Unlock()
	return activateScenarioLocked(s, cfg, sc)
}

// activateScenarioLocked is activateScenario for callers already holding
// s.activateMu.
func activateScenarioLocked(s *State, cfg Config, sc Scenario) bool {
	if s.isShuttingDown() {
		return false
	}
	s.reset()
	log.Printf("chaos: activating scenario %s", sc)

	switch sc {
	case ScenarioStable:
		// reset() already leaves the service in a clean STABLE state.
	case ScenarioOOMKill:
		s.startOOMGrowth(cfg.MemoryChunkSize)
		s.setScenario(ScenarioOOMKill)
	case ScenarioCPUBurn:
		s.startCPUBurn(cfg.CPUBurnThreads, cfg.CPUBurnDuration)
	case ScenarioSlowDeath:
		s.setHealthy(false)
		s.setScenario(ScenarioSlowDeath)
	case ScenarioCrash:
		s.setScenario(ScenarioCrash)
		if !s.crash() {
			return false
		}
	case ScenarioSlowResponse:
		s.setSlowResponse(true)
		s.setScenario(ScenarioSlowResponse)
	case ScenarioReadinessFlap:
		s.startFlap(cfg.ReadinessFlapInterval)
		s.setScenario(ScenarioReadinessFlap)
	}
	return true
}

// resetScenario is POST /chaos/reset. It stays allowed during shutdown (it
// only stops chaos); the probes remain red via shuttingDown.
func resetScenario(s *State) {
	s.activateMu.Lock()
	defer s.activateMu.Unlock()
	s.reset()
}
