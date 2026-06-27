package netcheck

import (
	"context"
	"errors"
	"testing"
)

// fakeDocker returns a fixed Container (or error).
type fakeDocker struct {
	c   Container
	err error
}

func (f fakeDocker) Inspect(context.Context, string) (Container, error) { return f.c, f.err }

// fakeProber replies for the addresses in ok and errors for everything else.
type fakeProber struct{ ok map[string]int }

func (f fakeProber) Probe(_ context.Context, addr string) (int, error) {
	if n, hit := f.ok[addr]; hit {
		return n, nil
	}
	return 0, errors.New("connection refused")
}

func statusOf(r Report, name string) Status {
	for _, c := range r.Checks {
		if c.Name == name {
			return c.Status
		}
	}
	return ""
}

var running = Container{Found: true, Running: true, Image: "worldvista/vehu"}

func cfg() Config {
	return Config{Container: "vehu", BrokerPort: 9430, RelayAddr: "0.0.0.0:19431", GuestHost: "10.0.2.2"}
}

func TestLoopbackPublishWithRelayUp(t *testing.T) {
	c := running
	c.Publish = []PortBinding{{ContainerPort: 9430, HostIP: "127.0.0.1", HostPort: 9430}}
	pr := fakeProber{ok: map[string]int{"127.0.0.1:9430": 52, "127.0.0.1:19431": 52}}

	r := Run(context.Background(), fakeDocker{c: c}, pr, cfg())

	if !r.OK {
		t.Fatalf("expected OK, got checks %+v", r.Checks)
	}
	if !r.RelayNeeded {
		t.Fatal("loopback publish must set RelayNeeded")
	}
	if statusOf(r, "broker publish") != StatusWarn {
		t.Fatalf("loopback publish should warn, got %q", statusOf(r, "broker publish"))
	}
	if statusOf(r, "relay") != StatusOK {
		t.Fatalf("relay up should be ok, got %q", statusOf(r, "relay"))
	}
	if r.CPRSTarget != "10.0.2.2:19431" {
		t.Fatalf("CPRS target = %q, want 10.0.2.2:19431", r.CPRSTarget)
	}
}

func TestLoopbackPublishWithRelayDown(t *testing.T) {
	c := running
	c.Publish = []PortBinding{{ContainerPort: 9430, HostIP: "127.0.0.1", HostPort: 9430}}
	// broker answers, relay does not.
	pr := fakeProber{ok: map[string]int{"127.0.0.1:9430": 52}}

	r := Run(context.Background(), fakeDocker{c: c}, pr, cfg())

	if r.OK {
		t.Fatal("relay down must make the report not-OK")
	}
	if statusOf(r, "broker listener") != StatusOK {
		t.Fatalf("broker listener should be ok, got %q", statusOf(r, "broker listener"))
	}
	if statusOf(r, "relay") != StatusFail {
		t.Fatalf("relay should fail, got %q", statusOf(r, "relay"))
	}
}

func TestPublishedOnAllInterfacesNeedsNoRelay(t *testing.T) {
	c := running
	c.Publish = []PortBinding{{ContainerPort: 9430, HostIP: "0.0.0.0", HostPort: 9430}}
	pr := fakeProber{ok: map[string]int{"127.0.0.1:9430": 52}}

	r := Run(context.Background(), fakeDocker{c: c}, pr, cfg())

	if !r.OK {
		t.Fatalf("expected OK, got %+v", r.Checks)
	}
	if r.RelayNeeded {
		t.Fatal("0.0.0.0 publish must NOT need a relay")
	}
	if statusOf(r, "broker publish") != StatusOK {
		t.Fatalf("all-interfaces publish should be ok, got %q", statusOf(r, "broker publish"))
	}
	if statusOf(r, "relay") != StatusInfo {
		t.Fatalf("relay should be info (not needed), got %q", statusOf(r, "relay"))
	}
	if r.CPRSTarget != "10.0.2.2:9430" {
		t.Fatalf("CPRS target = %q, want 10.0.2.2:9430 (direct)", r.CPRSTarget)
	}
}

func TestBrokerNotPublished(t *testing.T) {
	c := running
	c.Publish = nil // broker port not published
	r := Run(context.Background(), fakeDocker{c: c}, fakeProber{}, cfg())
	if r.OK {
		t.Fatal("unpublished broker must be not-OK")
	}
	if statusOf(r, "broker publish") != StatusFail {
		t.Fatalf("want broker-publish fail, got %q", statusOf(r, "broker publish"))
	}
	// Ladder must stop before probing a port that isn't there.
	if statusOf(r, "broker listener") != "" {
		t.Fatal("should not probe a listener when nothing is published")
	}
}

func TestContainerNotRunning(t *testing.T) {
	r := Run(context.Background(), fakeDocker{c: Container{Found: true, Running: false}}, fakeProber{}, cfg())
	if r.OK || statusOf(r, "docker") != StatusFail {
		t.Fatalf("stopped container must fail the docker check, got %+v", r.Checks)
	}
}

func TestDockerUnavailable(t *testing.T) {
	r := Run(context.Background(), fakeDocker{err: errors.New("no docker")}, fakeProber{}, cfg())
	if r.OK || statusOf(r, "docker") != StatusFail {
		t.Fatalf("docker error must fail the docker check, got %+v", r.Checks)
	}
}
