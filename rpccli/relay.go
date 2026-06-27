package rpccli

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/vista-cloud-dev/clikit"
	"github.com/vista-cloud-dev/v-rpc/internal/relay"
)

const relayService = "v-rpc-relay.service"

// relayCmd republishes the loopback-bound VistA broker on a reachable interface
// so a VirtualBox guest (CPRS) can connect. It is the built-in, dependency-free
// replacement for the hand-run `socat` relay: Go stdlib forwarding, the backend
// discovered from Docker, and an optional persistent user service.
type relayCmd struct {
	Listen     string `help:"Host address to listen on ($VRPC_RELAY_ADDR)." default:"0.0.0.0:19431" placeholder:"HOST:PORT" env:"VRPC_RELAY_ADDR"`
	To         string `help:"Broker backend host:port. Default: discovered from --container, else 127.0.0.1:9430." placeholder:"HOST:PORT"`
	Container  string `help:"Container to discover the broker port from ($VRPC_CONTAINER)." default:"vehu" env:"VRPC_CONTAINER"`
	BrokerPort int    `help:"Broker container-side port (for discovery)." default:"9430"`

	Install   bool `help:"Install a persistent service (user systemd on Linux) and exit."`
	Uninstall bool `help:"Remove the persistent service and exit."`
	Status    bool `help:"Report whether the relay is installed/running and exit."`
}

func (c *relayCmd) Run(cc *clikit.Context) error {
	switch {
	case c.Status:
		return c.runStatus(cc)
	case c.Uninstall:
		return c.runUninstall(cc)
	case c.Install:
		return c.runInstall(cc)
	default:
		return c.runForeground(cc)
	}
}

// backend resolves the address to forward to: an explicit --to, else the Docker
// publish binding, else the documented default.
func (c *relayCmd) backend(ctx context.Context) string {
	if c.To != "" {
		return c.To
	}
	if got := discoverBackend(ctx, c.Container, c.BrokerPort); got != "" {
		return got
	}
	return "127.0.0.1:9430"
}

// runForeground listens and forwards until Ctrl-C.
func (c *relayCmd) runForeground(cc *clikit.Context) error {
	to := c.backend(context.Background())
	ln, err := net.Listen("tcp", c.Listen)
	if err != nil {
		return clikit.Fail(clikit.ExitRuntime, "LISTEN", err.Error(),
			"is another relay already bound to "+c.Listen+"? try `v rpc relay --status`")
	}
	fmt.Fprintf(cc.Stderr, "# relay %s -> %s — Ctrl-C to stop\n", c.Listen, to)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := relay.Serve(ctx, ln, to); err != nil {
		return clikit.Fail(clikit.ExitRuntime, "RELAY", err.Error(), "")
	}
	return nil
}

// ensureRelayService writes the user systemd unit and enables+starts it
// (Linux only). Shared by `relay --install` and `doctor --fix`.
func ensureRelayService(listen, to string) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	unitPath, err := writeUserUnit(exe, listen, to)
	if err != nil {
		return "", err
	}
	if err := systemctlUser("daemon-reload"); err != nil {
		return "", err
	}
	if err := systemctlUser("enable", "--now", relayService); err != nil {
		return "", fmt.Errorf("%w (check `systemctl --user status %s`)", err, relayService)
	}
	return unitPath, nil
}

// runInstall writes + enables a persistent service. Linux uses a user systemd
// unit; other OSes get printed instructions (no silent host changes).
func (c *relayCmd) runInstall(cc *clikit.Context) error {
	to := c.backend(context.Background())
	if runtime.GOOS != "linux" {
		exe, _ := os.Executable()
		return cc.Result(
			struct {
				OS     string `json:"os"`
				Listen string `json:"listen"`
				To     string `json:"to"`
			}{runtime.GOOS, c.Listen, to},
			func() {
				fmt.Fprintf(cc.Stdout, "Auto-install is Linux-only. On %s, run this at login instead:\n  %s relay --listen %s --to %s\n",
					runtime.GOOS, exe, c.Listen, to)
			},
		)
	}
	unitPath, err := ensureRelayService(c.Listen, to)
	if err != nil {
		return clikit.Fail(clikit.ExitRuntime, "INSTALL", err.Error(), "")
	}
	return cc.Result(
		struct {
			Unit, Listen, To string
		}{unitPath, c.Listen, to},
		func() {
			fmt.Fprintf(cc.Stdout, "installed %s (%s -> %s), enabled + started.\n", relayService, c.Listen, to)
			fmt.Fprintln(cc.Stdout, "it now starts on boot. tip: `loginctl enable-linger` to run without a login session.")
		},
	)
}

func (c *relayCmd) runUninstall(cc *clikit.Context) error {
	if runtime.GOOS != "linux" {
		return clikit.Fail(clikit.ExitUsage, "UNSUPPORTED", "auto-uninstall is Linux-only", "")
	}
	_ = systemctlUser("disable", "--now", relayService) // best-effort
	unitPath, err := userUnitPath()
	if err == nil {
		_ = os.Remove(unitPath)
	}
	_ = systemctlUser("daemon-reload")
	return cc.Result(struct{ Removed string }{relayService},
		func() { fmt.Fprintf(cc.Stdout, "removed %s\n", relayService) })
}

func (c *relayCmd) runStatus(cc *clikit.Context) error {
	listening := portListening(c.Listen)
	installed, active := false, false
	if runtime.GOOS == "linux" {
		if p, err := userUnitPath(); err == nil {
			if _, statErr := os.Stat(p); statErr == nil {
				installed = true
			}
		}
		active = systemctlUser("is-active", "--quiet", relayService) == nil
	}
	return cc.Result(
		struct {
			Listen    string `json:"listen"`
			Listening bool   `json:"listening"`
			Installed bool   `json:"installed"`
			Active    bool   `json:"active"`
		}{c.Listen, listening, installed, active},
		func() {
			state := "not listening"
			if listening {
				state = "listening"
			}
			fmt.Fprintf(cc.Stdout, "relay on %s: %s; service installed=%t active=%t\n",
				c.Listen, state, installed, active)
		},
	)
}

// --- helpers ----------------------------------------------------------------

func userUnitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user", relayService), nil
}

func writeUserUnit(exe, listen, to string) (string, error) {
	path, err := userUnitPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	unit := fmt.Sprintf(`[Unit]
Description=v-rpc relay (host %s -> VistA broker %s) — republishes the loopback-bound broker for a VM client
After=network-online.target docker.service
Wants=network-online.target

[Service]
ExecStart=%s relay --listen %s --to %s
Restart=always
RestartSec=2

[Install]
WantedBy=default.target
`, listen, to, exe, listen, to)
	if err := os.WriteFile(path, []byte(unit), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func systemctlUser(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			return err
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// portListening reports whether something accepts TCP on the listen port,
// checked via loopback (a 0.0.0.0 listener answers on 127.0.0.1 too).
func portListening(listen string) bool {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return false
	}
	conn, err := net.Dial("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
