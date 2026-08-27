package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

type Scenario string

const (
	ScenarioStable        Scenario = "STABLE"
	ScenarioOOMKill       Scenario = "OOM_KILL"
	ScenarioCPUBurn       Scenario = "CPU_BURN"
	ScenarioSlowDeath     Scenario = "SLOW_DEATH"
	ScenarioCrash         Scenario = "CRASH"
	ScenarioSlowResponse  Scenario = "SLOW_RESPONSE"
	ScenarioReadinessFlap Scenario = "READINESS_FLAP"
)

// State holds all mutable chaos state, guarded by mu where noted.
type State struct {
	mu           sync.Mutex
	scenario     Scenario
	healthy      bool
	ready        bool
	slowResponse bool
	memBallast   [][]byte
	oomRunning   bool

	oomCancel  context.CancelFunc
	cpuCancel  context.CancelFunc
	flapCancel context.CancelFunc

	rootRequests uint64 // atomic counter for "/" 500-every-third-request behavior
	startTime    time.Time
}

func newState() *State {
	return &State{
		scenario:  ScenarioStable,
		healthy:   true,
		ready:     true,
		startTime: time.Now(),
	}
}

// reset cancels any running chaos goroutines and returns to a clean STABLE state.
func (s *State) reset() {
	s.mu.Lock()
	if s.oomCancel != nil {
		s.oomCancel()
		s.oomCancel = nil
	}
	if s.cpuCancel != nil {
		s.cpuCancel()
		s.cpuCancel = nil
	}
	if s.flapCancel != nil {
		s.flapCancel()
		s.flapCancel = nil
	}
	s.memBallast = nil
	s.oomRunning = false
	s.slowResponse = false
	s.healthy = true
	s.ready = true
	s.scenario = ScenarioStable
	s.mu.Unlock()
}

func (s *State) setScenario(sc Scenario) {
	s.mu.Lock()
	s.scenario = sc
	s.mu.Unlock()
}

func (s *State) getScenario() Scenario {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.scenario
}

func (s *State) isHealthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.healthy
}

func (s *State) setHealthy(v bool) {
	s.mu.Lock()
	s.healthy = v
	s.mu.Unlock()
}

func (s *State) toggleHealthy() bool {
	s.mu.Lock()
	s.healthy = !s.healthy
	v := s.healthy
	s.mu.Unlock()
	return v
}

func (s *State) isReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready
}

func (s *State) setReady(v bool) {
	s.mu.Lock()
	s.ready = v
	s.mu.Unlock()
}

func (s *State) isSlowResponse() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.slowResponse
}

func (s *State) setSlowResponse(v bool) {
	s.mu.Lock()
	s.slowResponse = v
	s.mu.Unlock()
}

func (s *State) memBallastBytes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, chunk := range s.memBallast {
		total += len(chunk)
	}
	return total
}

func (s *State) appendMemChunk(chunk []byte) {
	s.mu.Lock()
	s.memBallast = append(s.memBallast, chunk)
	s.mu.Unlock()
}

// startOOMGrowth starts (if not already running) a goroutine that appends one
// memory chunk per second, simulating gradual memory exhaustion.
func (s *State) startOOMGrowth(chunkSize int) {
	s.mu.Lock()
	if s.oomRunning {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.oomCancel = cancel
	s.oomRunning = true
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				chunk := make([]byte, chunkSize)
				for i := range chunk {
					chunk[i] = 1
				}
				s.appendMemChunk(chunk)
			}
		}
	}()
}

// startCPUBurn spins up n goroutines that saturate a CPU core each for the
// given duration (or until reset cancels them).
func (s *State) startCPUBurn(n int, duration time.Duration) {
	s.mu.Lock()
	if s.cpuCancel != nil {
		s.cpuCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	s.cpuCancel = cancel
	s.mu.Unlock()

	for i := 0; i < n; i++ {
		go burnCPU(ctx)
	}
}

func burnCPU(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		for i := 0; i < 1_000_000; i++ {
			_ = i * i
		}
	}
}

// startFlap starts (if not already running) a goroutine toggling readiness
// on the given interval.
func (s *State) startFlap(interval time.Duration) {
	s.mu.Lock()
	if s.flapCancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.flapCancel = cancel
	s.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.mu.Lock()
				s.ready = !s.ready
				s.mu.Unlock()
			}
		}
	}()
}

func (s *State) nextRootRequestCount() uint64 {
	return atomic.AddUint64(&s.rootRequests, 1)
}

func (s *State) uptime() time.Duration {
	return time.Since(s.startTime)
}
