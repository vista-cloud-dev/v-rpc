package rpccli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/vista-cloud-dev/v-rpc-debug/internal/netcheck"
	"github.com/vista-cloud-dev/v-rpc-debug/internal/xwbwire"
)

// xwbProber is the real netcheck.Prober: it dials a broker (directly or through
// the relay) and sends one no-arg [XWB] RPC. A live broker logs the name then
// replies (often a reject) — any reply proves the listener is up. It is a pure
// RPC-client probe (the same wire path as `ping`), not the engine driver seam.
type xwbProber struct{ timeout time.Duration }

func (p xwbProber) Probe(ctx context.Context, addr string) (int, error) {
	d := net.Dialer{Timeout: p.timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	if _, err := conn.Write(xwbwire.RPCMessage("XWB IM HERE")); err != nil {
		return 0, err
	}
	_ = conn.SetReadDeadline(time.Now().Add(p.timeout))
	buf := make([]byte, 512)
	n, _ := conn.Read(buf) // a reply (often a reject) or a timeout
	if n == 0 {
		// Connection accepted (e.g. the docker proxy) but no broker answered —
		// the M listener is not actually serving on this port.
		return 0, errors.New("connection accepted but the broker did not reply")
	}
	return n, nil
}

// dockerInspect is the real netcheck.Docker: it shells `docker inspect` (NOT
// `docker exec` — this never enters the engine) and extracts running state,
// image, and the host publish bindings for the broker container-port.
type dockerInspect struct{ brokerPort int }

func (d dockerInspect) Inspect(ctx context.Context, container string) (netcheck.Container, error) {
	out, err := exec.CommandContext(ctx, "docker", "inspect", container).CombinedOutput()
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return netcheck.Container{}, errors.New("docker is not installed or not on PATH")
		}
		msg := string(out)
		if strings.Contains(strings.ToLower(msg), "no such object") {
			return netcheck.Container{Found: false}, nil // ran fine; container just absent
		}
		// Collapse stdout `[]` + multi-line stderr into one tidy line.
		return netcheck.Container{}, fmt.Errorf("docker inspect: %s", strings.Join(strings.Fields(msg), " "))
	}
	var raw []struct {
		State           struct{ Running bool }
		Config          struct{ Image string }
		NetworkSettings struct {
			Ports map[string][]struct {
				HostIP   string `json:"HostIp"`
				HostPort string `json:"HostPort"`
			}
		}
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return netcheck.Container{}, fmt.Errorf("parse docker inspect: %w", err)
	}
	if len(raw) == 0 {
		return netcheck.Container{Found: false}, nil
	}
	r := raw[0]
	c := netcheck.Container{Found: true, Running: r.State.Running, Image: r.Config.Image}
	for _, b := range r.NetworkSettings.Ports[fmt.Sprintf("%d/tcp", d.brokerPort)] {
		hp, _ := strconv.Atoi(b.HostPort)
		c.Publish = append(c.Publish, netcheck.PortBinding{
			ContainerPort: d.brokerPort, HostIP: b.HostIP, HostPort: hp,
		})
	}
	return c, nil
}

// discoverBackend returns the host-side broker address (127.0.0.1:HOSTPORT) read
// from the container's publish binding, or "" if it can't be determined.
func discoverBackend(ctx context.Context, container string, brokerPort int) string {
	c, err := dockerInspect{brokerPort: brokerPort}.Inspect(ctx, container)
	if err != nil || !c.Found {
		return ""
	}
	for _, pb := range c.Publish {
		if pb.ContainerPort == brokerPort {
			return net.JoinHostPort("127.0.0.1", strconv.Itoa(pb.HostPort))
		}
	}
	return ""
}
