package relay

import (
	"context"
	"io"
	"net"
	"testing"
	"time"
)

// echoServer is a tiny TCP backend that echoes whatever it receives — it stands
// in for the broker so a relay test needs no engine.
func echoServer(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) { defer c.Close(); _, _ = io.Copy(c, c) }(c)
		}
	}()
	return ln
}

func TestServeForwardsBidirectional(t *testing.T) {
	backend := echoServer(t)
	defer backend.Close()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("relay listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = Serve(ctx, ln, backend.Addr().String()) }()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	defer c.Close()

	want := []byte("XWB IM HERE")
	if _, err := c.Write(want); err != nil {
		t.Fatalf("write: %v", err)
	}
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	got := make([]byte, len(want))
	if _, err := io.ReadFull(c, got); err != nil {
		t.Fatalf("read back through relay: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("relay corrupted payload: got %q want %q", got, want)
	}
}

func TestServeStopsOnContextCancel(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Serve(ctx, ln, "127.0.0.1:1") }()

	cancel() // canceling must close the listener and return nil (clean stop)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve returned %v on cancel, want nil", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after context cancel")
	}
}

func TestServeBackendRefusedClosesClient(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// 127.0.0.1:1 — nothing listens there, so the backend dial is refused.
	go func() { _ = Serve(ctx, ln, "127.0.0.1:1") }()

	c, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("dial relay: %v", err)
	}
	defer c.Close()
	// With no backend, the relay must close the client connection promptly
	// rather than hang — a read should hit EOF, not block forever.
	_ = c.SetReadDeadline(time.Now().Add(3 * time.Second))
	buf := make([]byte, 1)
	if _, err := c.Read(buf); err == nil {
		t.Fatal("expected client connection to be closed when backend is refused")
	}
}
