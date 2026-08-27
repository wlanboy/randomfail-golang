package main

import (
	"log"
	"math/rand"
	"os"
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
// CHAOS_INTERVAL seconds.
func runChaosCycle(s *State, cfg Config) {
	time.Sleep(cfg.ChaosStartupDelay)

	for {
		sc := autoScenarios[rand.Intn(len(autoScenarios))]
		activateScenario(s, cfg, sc)
		time.Sleep(cfg.ChaosInterval)
	}
}

func activateScenario(s *State, cfg Config, sc Scenario) {
	s.reset()
	log.Printf("chaos cycle: activating scenario %s", sc)

	switch sc {
	case ScenarioStable:
		// reset() already leaves the service in a clean STABLE state.
	case ScenarioOOMKill:
		s.startOOMGrowth(cfg.MemoryChunkSize)
		s.setScenario(ScenarioOOMKill)
	case ScenarioCPUBurn:
		s.startCPUBurn(cfg.CPUBurnThreads, cfg.CPUBurnDuration)
		s.setScenario(ScenarioCPUBurn)
	case ScenarioSlowDeath:
		s.setHealthy(false)
		s.setScenario(ScenarioSlowDeath)
	case ScenarioCrash:
		log.Println("chaos cycle: CRASH scenario triggered: exiting immediately")
		os.Exit(1)
	case ScenarioSlowResponse:
		s.setSlowResponse(true)
		s.setScenario(ScenarioSlowResponse)
	case ScenarioReadinessFlap:
		s.startFlap(cfg.ReadinessFlapInterval)
		s.setScenario(ScenarioReadinessFlap)
	}
}
