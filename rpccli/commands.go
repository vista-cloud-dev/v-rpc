// Package rpccli is the importable command surface of the `v rpc` domain. The
// standalone v-rpc binary mounts it at the top level; the `v` umbrella mounts
// the same structs as `v rpc <verb>` (the static-pinned composition v-pkg uses).
//
// Today it carries one group, `v rpc debug`, which taps the RPC Broker's native
// XWBDEBUG log over the m engine seam to view or save live RPC traffic. `v rpc`
// is scoped to grow other verbs later, so debug capture lives under `debug`.
package rpccli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/vista-cloud-dev/clikit"
	mdriver "github.com/vista-cloud-dev/m-driver-sdk"
	"github.com/vista-cloud-dev/v-rpc/internal/capture"
)

// Commands is the `v rpc` verb set, embedded by the umbrella and the standalone.
type Commands struct {
	Debug debugCmd `cmd:"" help:"Tap the RPC Broker's native XWBDEBUG log: view or save live RPC traffic."`
}

// engineConn selects which engine to drive and over which transport — the same
// neutral knobs as v-pkg/`m vista`. The connection (container/base-url,
// credentials) is read by the driver from its M_<ENGINE>_* environment; the
// optional --container is a convenience that sets M_<ENGINE>_CONTAINER for this
// process. Engine is required: ydb/vehu now, IRIS-VistA for VA validation later.
type engineConn struct {
	Engine    string `help:"Engine to reach: ydb or iris ($VRPC_ENGINE)." enum:"ydb,iris" required:"" env:"VRPC_ENGINE"`
	Transport string `help:"Driver transport: local | docker | remote ($VRPC_TRANSPORT)." enum:"local,docker,remote" default:"docker" env:"VRPC_TRANSPORT"`
	Container string `help:"Engine container/instance name; sets M_<ENGINE>_CONTAINER ($VRPC_CONTAINER)." placeholder:"NAME" env:"VRPC_CONTAINER"`
}

// execer resolves the m-<engine> driver (driver-contract §4) and returns the
// capture.Execer backed by the shared reference Client — the seam's single
// transport (waterline rule 3). v-rpc never hand-rolls transport.
func (e engineConn) execer() (capture.Execer, *clikit.Error) {
	if e.Container != "" {
		_ = os.Setenv("M_"+strings.ToUpper(e.Engine)+"_CONTAINER", e.Container)
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
