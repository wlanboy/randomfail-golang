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
	cpuGen     uint64 // identifies the current CPU burn so a stale timeout can't reset a newer one

	// shuttingDown is set once SIGTERM arrives; from then on the probes
	// report unhealthy/not-ready regardless of reset() or manual toggles.
	shuttingDown bool

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
	return s.healthy && !s.shuttingDown
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
	return s.ready && !s.shuttingDown
}

// beginShutdown permanently marks the service unhealthy and not ready.
func (s *State) beginShutdown() {
	s.mu.Lock()
	s.shuttingDown = true
	s.mu.Unlock()
}

func (s *State) isShuttingDown() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.shuttingDown
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

// appendMemChunkIfActive appends chunk only while ctx is still live, so a
// growth tick racing with reset() can't leave ballast behind.
func (s *State) appendMemChunkIfActive(ctx context.Context, chunk []byte) {
	s.mu.Lock()
	if ctx.Err() == nil {
		s.memBallast = append(s.memBallast, chunk)
	}
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
				s.appendMemChunkIfActive(ctx, chunk)
			}
		}
	}()
}

// startCPUBurn sets the CPU_BURN scenario and spins up n goroutines that
// saturate a CPU core each for the given duration (or until reset cancels
// them). When the duration elapses the scenario falls back to STABLE.
func (s *State) startCPUBurn(n int, duration time.Duration) {
	s.mu.Lock()
	if s.cpuCancel != nil {
		s.cpuCancel()
	}
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	s.cpuCancel = cancel
	s.cpuGen++
	gen := s.cpuGen
	s.scenario = ScenarioCPUBurn
	s.mu.Unlock()

	for i := 0; i < n; i++ {
		go burnCPU(ctx)
	}

	go func() {
		<-ctx.Done()
		if ctx.Err() != context.DeadlineExceeded {
			return // cancelled by reset() or a newer burn
		}
		s.mu.Lock()
		if s.cpuGen == gen {
			s.cpuCancel = nil
			if s.scenario == ScenarioCPUBurn {
				s.scenario = ScenarioStable
			}
		}
		s.mu.Unlock()
	}()
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
