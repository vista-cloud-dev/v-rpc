// Package netcheck is the pure check ladder behind `v rpc-debug doctor`. It walks the
// CPRS→VistA connection path — Docker container, broker publish mode, broker
// listener, host relay — and returns one structured Check per hop, each with a
// plain-language detail and the exact remediation. The whole ladder is pure
// logic over two injected seams (Docker inspect + a TCP/[XWB] Prober), so it is
// fully unit-testable with no engine, no Docker, and no network. The real
// adapters (docker inspect; xwbwire dial) live in the rpccli layer.
package netcheck

import (
	"context"
	"fmt"
	"net"
	"strconv"
)

// Status is a check outcome, mapped by the renderer to ✓/✗/⚠/ℹ.
type Status string

const (
	StatusOK   Status = "ok"   // ✓ this hop is healthy
	StatusFail Status = "fail" // ✗ this hop is broken — see Fix
	StatusWarn Status = "warn" // ⚠ works, but worth knowing
	StatusInfo Status = "info" // ℹ informational (not pass/fail)
)

// Check is one hop in the connection chain.
type Check struct {
	Name   string `json:"name"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
}

// PortBinding is one host-side publish of a container port (from docker inspect).
type PortBinding struct {
	ContainerPort int    `json:"containerPort"`
	HostIP        string `json:"hostIp"` // "127.0.0.1" (loopback) or "0.0.0.0"/"" (all)
	HostPort      int    `json:"hostPort"`
}

// Container is the inspected runtime state the ladder needs.
type Container struct {
	Found   bool          `json:"found"`
	Running bool          `json:"running"`
	Image   string        `json:"image"`
	Publish []PortBinding `json:"publish"` // bindings for the broker container-port
}

// Docker inspects a container's runtime state (real impl shells `docker inspect`).
type Docker interface {
	Inspect(ctx context.Context, container string) (Container, error)
}

// Prober dials a TCP address and performs a minimal [XWB] handshake, returning
// the number of reply bytes (a positive count proves a live broker listener).
type Prober interface {
	Probe(ctx context.Context, addr string) (replyBytes int, err error)
}

// Config holds the topology knobs (all discoverable or documented defaults).
type Config struct {
	Container  string // e.g. "vehu"
	BrokerPort int    // container-side broker port (default 9430)
	RelayAddr  string // host listen addr for the relay (default 0.0.0.0:19431)
	GuestHost  string // how the VM names the host (VBox NAT: 10.0.2.2)
}

// Report is the ladder's verdict plus the derived guidance.
type Report struct {
	Checks      []Check `json:"checks"`
	OK          bool    `json:"ok"`          // every non-info check passed
	RelayNeeded bool    `json:"relayNeeded"` // broker is loopback-bound
	CPRSTarget  string  `json:"cprsTarget"`  // what to type into CPRS (host:port)
}

// Run walks the ladder. It never returns an error — every condition becomes a
// Check so the caller can render the whole chain even when an early hop fails.
func Run(ctx context.Context, dk Docker, pr Prober, cfg Config) Report {
	if cfg.BrokerPort == 0 {
		cfg.BrokerPort = 9430
	}
	rep := Report{}
	add := func(c Check) { rep.Checks = append(rep.Checks, c) }

	// 1 — Docker / container.
	c, err := dk.Inspect(ctx, cfg.Container)
	switch {
	case err != nil:
		add(Check{"docker", StatusFail, "cannot inspect Docker: " + err.Error(),
			"is Docker installed and running? (`docker ps`)"})
		return finish(rep, cfg, "")
	case !c.Found:
		add(Check{"docker", StatusFail, fmt.Sprintf("container %q not found", cfg.Container),
			"start the VistA container, or pass --container NAME"})
		return finish(rep, cfg, "")
	case !c.Running:
		add(Check{"docker", StatusFail, fmt.Sprintf("container %q is not running", cfg.Container),
			"docker start " + cfg.Container})
		return finish(rep, cfg, "")
	default:
		add(Check{"docker", StatusOK,
			fmt.Sprintf("%s is running (image %s)", cfg.Container, c.Image), ""})
	}

	// 2 — Broker publish mode (the check that explains the whole failure).
	pb, found := brokerBinding(c.Publish, cfg.BrokerPort)
	if !found {
		add(Check{"broker publish", StatusFail,
			fmt.Sprintf("broker port %d is not published to the host", cfg.BrokerPort),
			"re-run the container publishing the broker port (e.g. -p 0.0.0.0:" +
				strconv.Itoa(cfg.BrokerPort) + ":" + strconv.Itoa(cfg.BrokerPort) + ")"})
		return finish(rep, cfg, "")
	}
	loopback := isLoopback(pb.HostIP)
	rep.RelayNeeded = loopback
	if loopback {
		add(Check{"broker publish", StatusWarn,
			fmt.Sprintf("published on %s:%d — bound to loopback, so a VM cannot reach it directly", pb.HostIP, pb.HostPort),
			"a relay is required: `v rpc-debug relay` (or republish the container on 0.0.0.0 to drop the relay)"})
	} else {
		add(Check{"broker publish", StatusOK,
			fmt.Sprintf("published on %s:%d — reachable from a VM directly, no relay needed", hostLabel(pb.HostIP), pb.HostPort), ""})
	}

	// 3 — Broker listener live (probe the host-published port directly).
	brokerProbe := net.JoinHostPort("127.0.0.1", strconv.Itoa(pb.HostPort))
	if n, perr := pr.Probe(ctx, brokerProbe); perr != nil {
		add(Check{"broker listener", StatusFail,
			"no [XWB] reply on " + brokerProbe + ": " + perr.Error(),
			"the broker job may be down inside VistA (TaskMan/XWB listener)"})
	} else {
		add(Check{"broker listener", StatusOK,
			fmt.Sprintf("[XWB] handshake on %s replied %d bytes (listener live)", brokerProbe, n), ""})
	}

	// 4 — Relay (only when the broker is loopback-bound). Probing the relay
	// addr runs the same handshake *through* it, proving the full host path.
	if rep.RelayNeeded {
		relayProbe, perr := localProbeAddr(cfg.RelayAddr)
		if perr != nil {
			add(Check{"relay", StatusFail, "bad relay address " + cfg.RelayAddr + ": " + perr.Error(),
				"pass --relay HOST:PORT (default 0.0.0.0:19431)"})
		} else if n, err := pr.Probe(ctx, relayProbe); err != nil {
			add(Check{"relay", StatusFail, "nothing forwarding on " + cfg.RelayAddr,
				"run `v rpc-debug relay` (or `v rpc-debug relay --install` for always-on)"})
			_ = n
		} else {
			add(Check{"relay", StatusOK,
				fmt.Sprintf("relay on %s forwards to the broker ([XWB] replied %d bytes)", cfg.RelayAddr, n), ""})
		}
	} else {
		add(Check{"relay", StatusInfo, "not needed — broker is reachable on all interfaces", ""})
	}

	// Derive the CPRS target: through the relay when loopback-bound, else direct.
	target := cprsTarget(cfg, pb, rep.RelayNeeded)
	return finish(rep, cfg, target)
}

// finish computes OK (no failing checks) and the CPRS-target guidance line.
func finish(rep Report, _ Config, target string) Report {
	rep.OK = true
	for _, c := range rep.Checks {
		if c.Status == StatusFail {
			rep.OK = false
		}
	}
	rep.CPRSTarget = target
	return rep
}

// brokerBinding returns the host binding for the broker container-port.
func brokerBinding(pbs []PortBinding, brokerPort int) (PortBinding, bool) {
	for _, pb := range pbs {
		if pb.ContainerPort == brokerPort {
			return pb, true
		}
	}
	return PortBinding{}, false
}

// isLoopback reports whether a publish HostIP confines the port to the host.
func isLoopback(hostIP string) bool {
	if hostIP == "" {
		return false // Docker treats "" as 0.0.0.0 (all interfaces)
	}
	ip := net.ParseIP(hostIP)
	return ip != nil && ip.IsLoopback()
}

func hostLabel(hostIP string) string {
	if hostIP == "" {
		return "0.0.0.0"
	}
	return hostIP
}

// localProbeAddr turns a listen address ("0.0.0.0:19431", ":19431") into one a
// host probe can dial (127.0.0.1:19431).
func localProbeAddr(listenAddr string) (string, error) {
	_, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort("127.0.0.1", port), nil
}

// cprsTarget is the host:port to type into CPRS: the relay port when the broker
// is loopback-bound, otherwise the broker's own published port — both via the
// guest's name for the host (VBox NAT: 10.0.2.2).
func cprsTarget(cfg Config, pb PortBinding, relayNeeded bool) string {
	host := cfg.GuestHost
	if host == "" {
		host = "10.0.2.2"
	}
	port := pb.HostPort
	if relayNeeded {
		if _, p, err := net.SplitHostPort(cfg.RelayAddr); err == nil {
			if n, err := strconv.Atoi(p); err == nil {
				port = n
			}
		}
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
