package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// shutdownTimeout bounds how long srv.Shutdown waits for in-flight requests
// after the SIGTERM delay. If the pod's terminationGracePeriodSeconds runs
// out first, the kubelet SIGKILLs the process anyway.
const shutdownTimeout = 10 * time.Second

func main() {
	cfg := loadConfig()
	s := newState()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: newMux(s, cfg),
	}

	go runChaosCycle(s, cfg)

	shutdownDone := make(chan struct{})
	go handleSigterm(s, cfg, srv, shutdownDone)

	log.Printf("randomfail listening on %s (chaosEnabled=%t, chaosInterval=%s, startupDelay=%s)",
		srv.Addr, cfg.ChaosEnabled, cfg.ChaosInterval, cfg.ChaosStartupDelay)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}

	// ListenAndServe returns as soon as Shutdown starts; wait until in-flight
	// requests have been drained before letting the process exit.
	<-shutdownDone
}

// handleSigterm implements the SIGTERM_DELAY behavior: on receiving SIGTERM
// the service immediately marks itself unhealthy/not-ready (to stop new
// traffic), waits SIGTERM_DELAY seconds to simulate a slow graceful shutdown,
// and then shuts the HTTP server down gracefully so in-flight requests can
// finish instead of being cut off.
func handleSigterm(s *State, cfg Config, srv *http.Server, done chan<- struct{}) {
	defer close(done)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM)
	<-sigCh

	log.Printf("received SIGTERM: marking unhealthy, delaying shutdown by %s", cfg.SigtermDelay)
	s.beginShutdown()
	time.Sleep(cfg.SigtermDelay)

	log.Printf("SIGTERM delay elapsed, shutting down server (timeout %s)", shutdownTimeout)
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("graceful shutdown incomplete, closing remaining connections: %v", err)
		srv.Close()
	}
	log.Println("server stopped, exiting")
}
