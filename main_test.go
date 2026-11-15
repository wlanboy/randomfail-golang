package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"
)

// TestHandleSigtermDrainsInFlightRequests checks the SIGTERM_DELAY flow end to
// end: after the signal the probes turn red at once, the server keeps serving
// during the delay, and a request that is still running when the delay ends
// completes with 200 before the server stops.
func TestHandleSigtermDrainsInFlightRequests(t *testing.T) {
	s := newState()
	cfg := Config{SigtermDelay: 200 * time.Millisecond}

	started := make(chan struct{})
	release := make(chan struct{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		close(started)
		<-release
		w.Write([]byte("done"))
	})
	mux.Handle("/", newMux(s, cfg))

	// No keep-alive: the default client sometimes dials a spare connection that
	// never carries a request, and Shutdown only drops such StateNew
	// connections after 5s.
	client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}

	ts := httptest.NewServer(mux)
	defer ts.Close()
	// Runs before ts.Close, which would otherwise wait forever for the blocked
	// handler if the test fails early.
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()

	sigCh := make(chan os.Signal, 1)
	done := make(chan struct{})
	go handleSigterm(s, cfg, ts.Config, sigCh, done)

	type result struct {
		status int
		body   string
		err    error
	}
	slowRes := make(chan result, 1)
	go func() {
		resp, err := client.Get(ts.URL + "/slow")
		if err != nil {
			slowRes <- result{err: err}
			return
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		slowRes <- result{status: resp.StatusCode, body: string(b), err: err}
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("slow request never reached the handler")
	}

	sigCh <- syscall.SIGTERM

	// Probes must turn red well before the delay ends. The server still
	// accepts new connections during the delay, so the probe gets an answer.
	deadline := time.Now().Add(cfg.SigtermDelay / 2)
	for {
		if code := getStatus(t, client, ts.URL+"/readyz"); code == http.StatusServiceUnavailable {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("/readyz did not return 503 shortly after SIGTERM")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if code := getStatus(t, client, ts.URL+"/healthz"); code != http.StatusInternalServerError {
		t.Errorf("/healthz after SIGTERM: status = %d, want %d", code, http.StatusInternalServerError)
	}

	// Once the delay has passed, Shutdown is running but has to wait for the
	// open request.
	time.Sleep(cfg.SigtermDelay + 100*time.Millisecond)
	select {
	case <-done:
		t.Fatal("handleSigterm returned while a request was still in flight")
	default:
	}

	unblock()

	select {
	case r := <-slowRes:
		if r.err != nil {
			t.Fatalf("slow request failed: %v", r.err)
		}
		if r.status != http.StatusOK || r.body != "done" {
			t.Errorf("slow request: status = %d, body = %q, want 200 %q", r.status, r.body, "done")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("slow request did not complete after release")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleSigterm did not finish after the last request completed")
	}

	if _, err := client.Get(ts.URL + "/readyz"); err == nil {
		t.Error("server still accepts requests after shutdown")
	}
}

func getStatus(t *testing.T, client *http.Client, url string) int {
	t.Helper()
	resp, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	resp.Body.Close()
	return resp.StatusCode
}
