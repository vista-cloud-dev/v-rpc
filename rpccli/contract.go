package rpccli

import (
	"github.com/alecthomas/kong"

	"github.com/vista-cloud-dev/clikit"
	"github.com/vista-cloud-dev/v-pkg/vcontract"
)

const (
	// Version is the declared SemVer of the v-rpc-debug domain surface. It is a
	// committed constant (distinct from the link-time build version reported by
	// `version`) so the generated contract is drift-stable.
	Version = "0.3.0"

	// ContractVersion bumps only on an incompatible command-surface change
	// (v-cli-platform.md §4), independent of Version.
	ContractVersion = "2.0"
)

// Contract returns the v-rpc-debug domain contract manifest, reflected from the actual
// rpccli command tree. It is what the standalone `v-rpc-debug` can emit and what the
// `v` umbrella aggregates into its registry — one source, so the manifest can
// never drift from the real verbs.
func Contract() vcontract.Manifest {
	var grammar struct {
		clikit.Globals
		Commands
	}
	k, err := kong.New(&grammar)
	if err != nil {
		// The grammar is static; a failure here is a programming error.
		panic("rpccli: build contract grammar: " + err.Error())
	}
	return vcontract.Build("rpc-debug", Version, ContractVersion, k)
}
