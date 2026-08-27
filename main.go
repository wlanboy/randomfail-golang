package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	cfg := loadConfig()
	s := newState()

	go runChaosCycle(s, cfg)
	go handleSigterm(s, cfg)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: newMux(s, cfg),
	}

	log.Printf("randomfail listening on %s (chaosInterval=%s, startupDelay=%s)",
		srv.Addr, cfg.ChaosInterval, cfg.ChaosStartupDelay)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}

// handleSigterm implements the SIGTERM_DELAY behavior: on receiving SIGTERM
// the service immediately marks itself unhealthy/not-ready (to stop new
// traffic) and only exits after SIGTERM_DELAY seconds, simulating a slow
// graceful shutdown.
func handleSigterm(s *State, cfg Config) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	<-sigCh

	log.Printf("received SIGTERM: marking unhealthy, delaying shutdown by %s", cfg.SigtermDelay)
	s.setHealthy(false)
	s.setReady(false)
	time.Sleep(cfg.SigtermDelay)

	log.Println("SIGTERM delay elapsed, exiting")
	os.Exit(0)
}
