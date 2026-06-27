package rpccli

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/vista-cloud-dev/clikit"
	"github.com/vista-cloud-dev/v-rpc/internal/netcheck"
)

// doctorCmd walks the CPRS→VistA connection path and reports each hop with a
// plain-language diagnosis and the exact fix. It reuses the same two seams the
// rest of v-rpc uses: `docker inspect` (engine-side state) and an [XWB] socket
// probe (the `ping` wire path). With --fix it starts the host relay and prints
// the CPRS connection string. It never modifies VistA, the container, or the VM.
type doctorCmd struct {
	Container  string        `help:"VistA container to inspect ($VRPC_CONTAINER)." default:"vehu" env:"VRPC_CONTAINER"`
	BrokerPort int           `help:"Broker container-side port." default:"9430"`
	RelayAddr  string        `help:"Relay host address ($VRPC_RELAY_ADDR)." default:"0.0.0.0:19431" placeholder:"HOST:PORT" env:"VRPC_RELAY_ADDR"`
	GuestHost  string        `help:"How the VM names the host (VBox NAT = 10.0.2.2)." default:"10.0.2.2" placeholder:"HOST"`
	Timeout    time.Duration `help:"Per-probe TCP timeout." default:"3s"`
	Fix        bool          `help:"Start the relay if it's needed and missing, then re-check."`
}

func (c *doctorCmd) Run(cc *clikit.Context) error {
	ctx := context.Background()
	cfg := netcheck.Config{
		Container:  c.Container,
		BrokerPort: c.BrokerPort,
		RelayAddr:  c.RelayAddr,
		GuestHost:  c.GuestHost,
	}
	dk := dockerInspect{brokerPort: c.BrokerPort}
	pr := xwbProber{timeout: c.Timeout}

	rep := netcheck.Run(ctx, dk, pr, cfg)

	var fixNote string
	if c.Fix && rep.RelayNeeded && checkFailed(rep, "relay") {
		if runtime.GOOS != "linux" {
			fixNote = "--fix auto-install is Linux-only; run `v rpc relay` by hand"
		} else if _, err := ensureRelayService(c.RelayAddr, c.backendFor(ctx)); err != nil {
			fixNote = "could not start relay: " + err.Error()
		} else {
			fixNote = "started the relay service; re-checking…"
			rep = netcheck.Run(ctx, dk, pr, cfg) // re-run so the report reflects the fix
		}
	}

	return cc.Result(rep, func() { c.render(cc, rep, fixNote) })
}

// backendFor resolves the relay backend for --fix (discovered, else default).
func (c *doctorCmd) backendFor(ctx context.Context) string {
	if got := discoverBackend(ctx, c.Container, c.BrokerPort); got != "" {
		return got
	}
	return "127.0.0.1:9430"
}

// render prints the human ladder: one status line per hop, an indented fix under
// each failure, then the CPRS target.
func (c *doctorCmd) render(cc *clikit.Context, rep netcheck.Report, fixNote string) {
	for _, ck := range rep.Checks {
		line := fmt.Sprintf("%-16s %s", ck.Name, ck.Detail)
		switch ck.Status {
		case netcheck.StatusOK:
			fmt.Fprintln(cc.Stdout, cc.Success(line))
		case netcheck.StatusFail:
			fmt.Fprintln(cc.Stdout, cc.Failure(line))
		case netcheck.StatusWarn:
			fmt.Fprintln(cc.Stdout, cc.Warning(line))
		default:
			fmt.Fprintln(cc.Stdout, cc.Info(line))
		}
		if ck.Fix != "" && ck.Status != netcheck.StatusOK {
			fmt.Fprintf(cc.Stdout, "                 %s %s\n", cc.Glyphs().Arrow, cc.Accent(ck.Fix))
		}
	}
	if fixNote != "" {
		fmt.Fprintln(cc.Stdout, cc.Info(fixNote))
	}
	fmt.Fprintln(cc.Stdout)
	if rep.CPRSTarget != "" {
		host, port, _ := splitTarget(rep.CPRSTarget)
		fmt.Fprintf(cc.Stdout, "CPRS should connect to:  %s   (s=%s p=%s)\n",
			cc.Accent(rep.CPRSTarget), host, port)
	}
	if rep.OK {
		fmt.Fprintln(cc.Stdout, cc.Success("path looks good — CPRS should connect."))
	} else {
		fmt.Fprintln(cc.Stdout, cc.Failure("path is broken — fix the ✗ lines above (or re-run with --fix)."))
	}
}

// checkFailed reports whether the named check failed.
func checkFailed(rep netcheck.Report, name string) bool {
	for _, ck := range rep.Checks {
		if ck.Name == name {
			return ck.Status == netcheck.StatusFail
		}
	}
	return false
}

// splitTarget splits "host:port" for the CPRS s=/p= hint.
func splitTarget(target string) (host, port string, ok bool) {
	for i := len(target) - 1; i >= 0; i-- {
		if target[i] == ':' {
			return target[:i], target[i+1:], true
		}
	}
	return target, "", false
}
