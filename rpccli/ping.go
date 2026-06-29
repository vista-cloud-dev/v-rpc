package rpccli

import (
	"fmt"
	"net"
	"time"

	"github.com/vista-cloud-dev/clikit"
	"github.com/vista-cloud-dev/v-rpc-debug/internal/xwbwire"
)

// defaultPingRPCs are harmless no-arg RPCs the broker logs by name then rejects
// (no signed-on session) — enough to prove a tap is capturing.
var defaultPingRPCs = []string{"XWB IM HERE", "XUS INTRO MSG", "XWB GET VARIABLE VALUE"}

// pingCmd fires test RPCs directly at a broker's TCP port so a running
// `v rpc debug tail`/`capture` has traffic to capture. It connects as an RPC
// client over the [XWB] wire protocol (one fresh connection per RPC, as the
// broker may end the job after one unauthenticated message) — it does NOT reach
// the M engine, so it takes a broker --addr, not the engine flags.
type pingCmd struct {
	Addr    string        `help:"Broker host:port to ping; vehu=127.0.0.1:9430 ($VRPC_ADDR)." default:"127.0.0.1:9430" placeholder:"HOST:PORT" env:"VRPC_ADDR"`
	RPC     []string      `help:"RPC name to send; repeatable. Default: a small no-arg set." placeholder:"NAME"`
	Count   int           `help:"Send the whole set this many times." default:"1"`
	Timeout time.Duration `help:"Per-connection read timeout." default:"3s"`
}

type pingResult struct {
	RPC     string `json:"rpc"`
	Sent    bool   `json:"sent"`
	RespLen int    `json:"respLen"`
	Err     string `json:"err,omitempty"`
}

func (c *pingCmd) Run(cc *clikit.Context) error {
	names := c.RPC
	if len(names) == 0 {
		names = defaultPingRPCs
	}
	var results []pingResult
	for i := 0; i < c.Count; i++ {
		for _, name := range names {
			results = append(results, c.fire(name))
		}
	}
	sent := 0
	for _, r := range results {
		if r.Sent {
			sent++
		}
	}
	return cc.Result(
		struct {
			Addr    string       `json:"addr"`
			Sent    int          `json:"sent"`
			Total   int          `json:"total"`
			Results []pingResult `json:"results"`
		}{c.Addr, sent, len(results), results},
		func() {
			for _, r := range results {
				if r.Sent {
					fmt.Fprintf(cc.Stdout, "  sent %-26s (broker replied %d bytes)\n", r.RPC, r.RespLen)
				} else {
					fmt.Fprintf(cc.Stdout, "  FAILED %-24s %s\n", r.RPC, r.Err)
				}
			}
			fmt.Fprintf(cc.Stdout, "%d/%d RPC(s) sent to %s\n", sent, len(results), c.Addr)
		},
	)
}

// fire opens a fresh connection, sends one RPC, drains the reply, and closes.
func (c *pingCmd) fire(name string) pingResult {
	res := pingResult{RPC: name}
	conn, err := net.DialTimeout("tcp", c.Addr, c.Timeout)
	if err != nil {
		res.Err = err.Error()
		return res
	}
	defer conn.Close()
	if _, err := conn.Write(xwbwire.RPCMessage(name)); err != nil {
		res.Err = err.Error()
		return res
	}
	res.Sent = true
	_ = conn.SetReadDeadline(time.Now().Add(c.Timeout))
	buf := make([]byte, 512)
	n, _ := conn.Read(buf) // a reply (often a reject) or timeout — both are fine
	res.RespLen = n
	return res
}
