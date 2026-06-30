// Package rpccli is the importable command surface of the `v rpc-debug` domain.
// The standalone v-rpc-debug binary mounts it at the top level; the `v` umbrella
// mounts the same structs as `v rpc-debug <verb>` (the static-pinned composition
// v-pkg uses). It is the safe, read-only sibling of the high-risk `v rpc-tap`
// domain (v-rpc-tap repo); the two integrate only at the v-cli busybox.
//
// Two groups: Capture taps the RPC Broker's native XWBDEBUG log over the m
// engine seam to view or save live RPC traffic (tail/capture/status/arm/disarm/
// clear/ping); Connect diagnoses and republishes the CPRS↔VistA broker network
// path (doctor/relay).
package rpccli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vista-cloud-dev/clikit"
	mdriver "github.com/vista-cloud-dev/m-driver-sdk"
	"github.com/vista-cloud-dev/v-rpc-debug/internal/capture"
)

// Commands is the `v rpc-debug` verb set, embedded by the umbrella and the
// standalone. The XWBDEBUG tap verbs sit directly at the domain level (the
// names-only oracle; level 3 logs RPC params = PHI), grouped as Capture; the
// network verbs are grouped as Connect.
type Commands struct {
	Tail    tailCmd    `cmd:"" group:"Capture" help:"Stream live RPC traffic to the terminal (Ctrl-C to stop)." example:"v rpc-debug tail --container vehu"`
	Capture captureCmd `cmd:"" group:"Capture" help:"Append live RPC traffic to a file as LDJSON for offline analysis." example:"v rpc-debug capture --container vehu --out cprs.ldjson"`
	Status  statusCmd  `cmd:"" group:"Capture" help:"Show the current XWBDEBUG level and active log jobs." example:"v rpc-debug status --container vehu"`
	Arm     armCmd     `cmd:"" group:"Capture" help:"Turn XWBDEBUG capture on (set the broker debug level)." example:"v rpc-debug arm --container vehu --level 2"`
	Disarm  disarmCmd  `cmd:"" group:"Capture" help:"Turn XWBDEBUG capture off (restore the debug level)." example:"v rpc-debug disarm --container vehu --level 1"`
	Clear   clearCmd   `cmd:"" group:"Capture" help:"Wipe the buffered XWBLOG (leave the engine pristine)." example:"v rpc-debug clear --container vehu"`
	Ping    pingCmd    `cmd:"" group:"Capture" help:"Fire test RPCs at a broker so a tap has traffic to capture." example:"v rpc-debug ping --addr 127.0.0.1:9430"`

	Doctor doctorCmd `cmd:"" group:"Connect" help:"Diagnose the CPRS↔VistA broker network path (and --fix it)." example:"v rpc-debug doctor --container vehu"`
	Relay  relayCmd  `cmd:"" group:"Connect" help:"Republish the loopback-bound broker so a VM (CPRS) can reach it." example:"v rpc-debug relay --install --to 127.0.0.1:9430"`
}

// engineConn selects which engine to drive and over which transport — the same
// neutral knobs as v-pkg/`m vista`. The connection (container/base-url,
// credentials) is read by the driver from its M_<ENGINE>_* environment; the
// optional --container is a convenience that sets M_<ENGINE>_CONTAINER for this
// process. Engine is required: ydb/vehu now, IRIS-VistA for VA validation later.
type engineConn struct {
	Engine    string `help:"Engine to reach: ydb or iris ($VRPC_ENGINE)." enum:"ydb,iris" default:"ydb" env:"VRPC_ENGINE"`
	Transport string `help:"Driver transport: local | docker | remote ($VRPC_TRANSPORT)." enum:"local,docker,remote" default:"docker" env:"VRPC_TRANSPORT"`
	Container string `help:"Engine container/instance name; sets M_<ENGINE>_CONTAINER ($VRPC_CONTAINER)." placeholder:"NAME" env:"VRPC_CONTAINER"`
}

// execer resolves the m-<engine> driver (driver-contract §4) and returns the
// capture.Execer backed by the shared reference Client — the seam's single
// transport (waterline rule 3). v-rpc-debug never hand-rolls transport.
func (e engineConn) execer() (capture.Execer, *clikit.Error) {
	envKey := "M_" + strings.ToUpper(e.Engine) + "_CONTAINER"
	if e.Container != "" {
		_ = os.Setenv(envKey, e.Container)
	}
	// Docker transport execs M inside a named container, so the container name is
	// the one irreducible input (engine + transport both default). Surface its
	// absence as a USAGE error up front — not a later cryptic engine fault — so
	// clikit answers a bare `tail`/`status`/… with the verb's help.
	if e.Transport == "docker" && os.Getenv(envKey) == "" {
		return nil, clikit.Fail(clikit.ExitUsage, "USAGE",
			"no engine container: --engine "+e.Engine+" over docker transport needs a container name",
			"pass --container NAME or set $VRPC_CONTAINER (e.g. vehu for ydb, foia-t12 for iris)")
	}
	bin, err := mdriver.Locate(e.Engine, mdriver.DefaultLocateDeps())
	if err != nil {
		return nil, clikit.Fail(clikit.ExitRefused, "NO_DRIVER", err.Error(),
			"build the m-"+e.Engine+" driver (make build) or set M_"+strings.ToUpper(e.Engine)+"_BIN")
	}
	cl := mdriver.NewClient(bin, e.Engine, e.Transport, nil, nil)
	return mdriverExecer{cl: cl}, nil
}

// mdriverExecer adapts mdriver.Client.ExecEval to capture.Execer: a structured
// engine fault (EngineError) becomes a Go error so the command can report it.
type mdriverExecer struct{ cl *mdriver.Client }

func (m mdriverExecer) Exec(ctx context.Context, command string) (string, error) {
	res, err := m.cl.ExecEval(ctx, command)
	if err != nil {
		return "", err
	}
	if res.EngineError != nil {
		return "", fmt.Errorf("engine fault %s: %s", res.EngineError.Mnemonic, res.EngineError.Text)
	}
	return res.Stdout, nil
}
