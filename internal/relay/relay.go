// Package relay is a dependency-free TCP forwarder: it republishes a
// loopback-bound service (the VistA RPC broker, published by Docker on
// 127.0.0.1) on a reachable interface so a VirtualBox guest (CPRS) can connect.
// It replaces the ad-hoc `socat` relay with ~30 lines of Go stdlib, so the next
// user needs no external tool and it works the same on Linux/macOS/WSL.
//
// It is pure transport (bytes in, bytes out) — it does NOT reach the M engine
// and is not the driver seam; it is the same RPC-client-side plumbing as the
// `ping` verb, one layer down.
package relay

import (
	"context"
	"io"
	"net"
)

// Serve accepts connections on ln and forwards each to the backend address `to`,
// copying bytes in both directions until either side closes. One goroutine per
// connection. It returns nil when ctx is canceled (a clean stop) and a non-nil
// error only on an unexpected Accept failure.
func Serve(ctx context.Context, ln net.Listener, to string) error {
	// Closing the listener unblocks Accept; do it when the caller cancels.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	for {
		client, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil // canceled — clean stop
			}
			return err
		}
		go forward(ctx, client, to)
	}
}

// forward dials the backend and pipes client<->backend. If the backend dial
// fails (engine/broker down), it closes the client connection rather than hang,
// so a misconfigured path fails fast and visibly.
func forward(ctx context.Context, client net.Conn, to string) {
	defer client.Close()
	var d net.Dialer
	backend, err := d.DialContext(ctx, "tcp", to)
	if err != nil {
		return // client closed by defer — caller sees EOF immediately
	}
	defer backend.Close()

	// When either direction ends (or ctx cancels), returning closes both conns
	// via the defers, which unblocks the other io.Copy.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(backend, client); done <- struct{}{} }()
	go func() { _, _ = io.Copy(client, backend); done <- struct{}{} }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}
