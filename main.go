// Command v-rpc-debug is the standalone form of the `v rpc` domain — VistA RPC
// developer tools. Today it carries `v rpc debug`, which taps the RPC Broker's
// native XWBDEBUG log over the m engine seam (mdriver.Client) to view live RPC
// traffic in the terminal or save it to a file as LDJSON for offline comparison
// against the VSL tap. The verb set lives in the importable rpccli package so
// the `v` umbrella mounts the same commands as `v rpc <verb>`.
//
// Try:
//
//	v-rpc-debug debug status  --engine ydb --container vehu
//	v-rpc-debug debug tail    --engine ydb --container vehu
//	v-rpc-debug debug capture --engine ydb --container vehu --out rpc.ldjson
//	v-rpc-debug debug arm     --engine ydb --container vehu
//	v-rpc-debug debug disarm  --engine ydb --container vehu
//	v-rpc-debug schema | jq .
package main

import (
	"os"

	"github.com/willabides/kongplete"

	"github.com/vista-cloud-dev/clikit"
	"github.com/vista-cloud-dev/v-rpc-debug/rpccli"
)

// CLI is the standalone v-rpc-debug grammar: the rpccli verbs at the top level, plus
// the shared clikit meta commands.
type CLI struct {
	clikit.Globals
	rpccli.Commands

	Menu    clikit.MenuCmd    `cmd:"" help:"Browse the command surface interactively (palette)."`
	Schema  clikit.SchemaCmd  `cmd:"" hidden:"" help:"Emit the command/flag/enum tree as JSON (agent discovery)."`
	Version clikit.VersionCmd `cmd:"" help:"Show version and build info."`

	InstallCompletions kongplete.InstallCompletions `cmd:"" hidden:"" help:"Install shell tab-completions."`
}

func main() {
	cli := &CLI{}
	os.Exit(clikit.Run(
		"v-rpc-debug",
		"VistA RPC developer tools — tap the RPC Broker debug log (view / save).",
		cli, &cli.Globals,
	))
}
