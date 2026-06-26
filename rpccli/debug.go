package rpccli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/vista-cloud-dev/clikit"
	"github.com/vista-cloud-dev/v-rpc/internal/capture"
	"github.com/vista-cloud-dev/v-rpc/internal/xwblog"
)

// debugCmd groups the XWBDEBUG tap verbs. Capture is debug-grade and names-only
// at level 2 (level 3 logs RPC parameters = PHI); it lands in ^XTMP("XWBLOG"_$J)
// and is the zero-install oracle for the durable VSL tap, not a replacement.
type debugCmd struct {
	Tail    tailCmd    `cmd:"" help:"Stream live RPC traffic to the terminal (Ctrl-C to stop)."`
	Capture captureCmd `cmd:"" help:"Append live RPC traffic to a file as LDJSON for offline analysis."`
	Status  statusCmd  `cmd:"" help:"Show the current XWBDEBUG level and active log jobs."`
	Arm     armCmd     `cmd:"" help:"Turn XWBDEBUG capture on (set the broker debug level)."`
	Disarm  disarmCmd  `cmd:"" help:"Turn XWBDEBUG capture off (restore the debug level)."`
}

// --- shared capture options + loop ------------------------------------------

// tapOpts are the knobs common to tail and capture.
type tapOpts struct {
	All      bool          `help:"Show every log line, not just RPC: lines."`
	Filter   string        `help:"Only RPCs whose name contains this (case-insensitive)." placeholder:"TEXT"`
	Interval float64       `help:"Poll interval in seconds." default:"1.0"`
	Duration time.Duration `help:"Stop after this long (e.g. 30s); 0 = run until Ctrl-C." default:"0"`
	Level    int           `help:"XWBDEBUG level to arm: 2=names, 3=names+params (PHI)." default:"2" enum:"2,3"`
	Keep     bool          `help:"Leave XWBDEBUG armed on exit (default restores the prior level)."`
	NoClear  bool          `help:"Do not clear the existing XWBLOG on start (capture what is already buffered too)."`
}

func (o tapOpts) show(r capture.Record) bool {
	if !o.All && r.Kind != xwblog.KindRPC {
		return false
	}
	if o.Filter != "" {
		if r.RPC == "" || !strings.Contains(strings.ToUpper(r.RPC), strings.ToUpper(o.Filter)) {
			return false
		}
	}
	return true
}

// runTap arms XWBDEBUG, clears the log, then polls until Ctrl-C, handing each
// new record to emit. The prior level is restored on exit unless Keep is set.
// Restore uses a fresh context so it still runs after the poll context cancels.
func runTap(ec engineConn, o tapOpts, emit func(capture.Record), note func(string)) *clikit.Error {
	ex, ferr := ec.execer()
	if ferr != nil {
		return ferr
	}
	ctx := context.Background()
	prior, err := capture.Level(ctx, ex)
	if err != nil {
		return clikit.Fail(clikit.ExitRuntime, "ENGINE", err.Error(),
			"is the engine up and reachable over the driver? try `v rpc debug status`")
	}
	if err := capture.Arm(ctx, ex, o.Level); err != nil {
		return clikit.Fail(clikit.ExitRuntime, "ARM", err.Error(), "")
	}
	defer func() {
		if !o.Keep {
			_ = capture.Disarm(context.Background(), ex, prior)
		}
	}()
	if !o.NoClear {
		_ = capture.Clear(ctx, ex)
	}
	note(fmt.Sprintf("# XWBDEBUG armed at %d on %s; restoring to %d on exit — Ctrl-C to stop",
		o.Level, ec.Engine, prior))

	sctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()
	var deadline <-chan time.Time // nil channel blocks forever when Duration==0
	if o.Duration > 0 {
		deadline = time.After(o.Duration)
	}
	tl := capture.NewTailer()
	interval := time.Duration(o.Interval * float64(time.Second))
	for {
		recs, err := tl.ReadNew(sctx, ex)
		if err != nil && sctx.Err() == nil {
			note("# read error: " + err.Error())
		}
		for _, r := range recs {
			if o.show(r) {
				emit(r)
			}
		}
		select {
		case <-sctx.Done():
			note("\n# stopped")
			return nil
		case <-deadline:
			note("# duration reached")
			return nil
		case <-time.After(interval):
		}
	}
}

// humanLine is the terminal rendering of one record.
func humanLine(r capture.Record) string {
	return fmt.Sprintf("[%s] job %7s  %s", xwblog.HHMMSS(r.HTime), r.PID, r.Message)
}

// --- tail -------------------------------------------------------------------

type tailCmd struct {
	engineConn
	tapOpts
}

func (c *tailCmd) Run(cc *clikit.Context) error {
	emit := func(r capture.Record) {
		if cc.JSON() {
			fmt.Fprintln(cc.Stdout, r.LDJSON())
		} else {
			fmt.Fprintln(cc.Stdout, humanLine(r))
		}
	}
	note := func(s string) {
		if !cc.JSON() {
			fmt.Fprintln(cc.Stderr, s)
		}
	}
	if e := runTap(c.engineConn, c.tapOpts, emit, note); e != nil {
		return e
	}
	return nil
}

// --- capture ----------------------------------------------------------------

type captureCmd struct {
	engineConn
	tapOpts
	Out   string `help:"Output file (file://PATH or PATH); appended as LDJSON." required:"" placeholder:"PATH"`
	Quiet bool   `help:"Do not also echo captured RPCs to the terminal."`
}

func (c *captureCmd) Run(cc *clikit.Context) error {
	path := strings.TrimPrefix(c.Out, "file://")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return clikit.Fail(clikit.ExitRuntime, "OUTPUT", err.Error(), "check the --out path is writable")
	}
	defer f.Close()
	n := 0
	emit := func(r capture.Record) {
		fmt.Fprintln(f, r.LDJSON())
		n++
		if !c.Quiet {
			fmt.Fprintln(cc.Stderr, humanLine(r))
		}
	}
	note := func(s string) { fmt.Fprintln(cc.Stderr, s) }
	if e := runTap(c.engineConn, c.tapOpts, emit, note); e != nil {
		return e
	}
	return cc.Result(
		struct {
			File    string `json:"file"`
			Records int    `json:"records"`
			Engine  string `json:"engine"`
		}{path, n, c.Engine},
		func() { fmt.Fprintf(cc.Stdout, "wrote %d record(s) to %s\n", n, path) },
	)
}

// --- status -----------------------------------------------------------------

type statusCmd struct {
	engineConn
}

func (c *statusCmd) Run(cc *clikit.Context) error {
	ex, ferr := c.execer()
	if ferr != nil {
		return ferr
	}
	ctx := context.Background()
	lvl, err := capture.Level(ctx, ex)
	if err != nil {
		return clikit.Fail(clikit.ExitRuntime, "ENGINE", err.Error(),
			"is the engine up and reachable over the driver?")
	}
	recs, err := capture.ReadAll(ctx, ex)
	if err != nil {
		return clikit.Fail(clikit.ExitRuntime, "ENGINE", err.Error(), "")
	}
	jobs := map[string]struct{}{}
	rpcs := 0
	for _, r := range recs {
		jobs[r.Job] = struct{}{}
		if r.Kind == xwblog.KindRPC {
			rpcs++
		}
	}
	data := struct {
		Engine  string `json:"engine"`
		Level   int    `json:"level"`
		Armed   bool   `json:"armed"`
		LogJobs int    `json:"logJobs"`
		RPCs    int    `json:"rpcs"`
	}{c.Engine, lvl, lvl >= 2, len(jobs), rpcs}
	return cc.Result(data, func() {
		armed := "off"
		if data.Armed {
			armed = "ON"
		}
		fmt.Fprintf(cc.Stdout, "engine %s: XWBDEBUG level %d (%s); %d log job(s), %d RPC line(s) buffered\n",
			data.Engine, data.Level, armed, data.LogJobs, data.RPCs)
	})
}

// --- arm / disarm -----------------------------------------------------------

type armCmd struct {
	engineConn
	Level int `help:"XWBDEBUG level: 2=names, 3=names+params (PHI)." default:"2" enum:"2,3"`
}

func (c *armCmd) Run(cc *clikit.Context) error {
	ex, ferr := c.execer()
	if ferr != nil {
		return ferr
	}
	if err := capture.Arm(context.Background(), ex, c.Level); err != nil {
		return clikit.Fail(clikit.ExitRuntime, "ARM", err.Error(), "")
	}
	return cc.Result(
		struct {
			Engine string `json:"engine"`
			Level  int    `json:"level"`
		}{c.Engine, c.Level},
		func() { fmt.Fprintf(cc.Stdout, "XWBDEBUG armed at level %d on %s\n", c.Level, c.Engine) },
	)
}

type disarmCmd struct {
	engineConn
	Level int `help:"Level to restore XWBDEBUG to (1 = stock VistA default)." default:"1"`
}

func (c *disarmCmd) Run(cc *clikit.Context) error {
	ex, ferr := c.execer()
	if ferr != nil {
		return ferr
	}
	if err := capture.Disarm(context.Background(), ex, c.Level); err != nil {
		return clikit.Fail(clikit.ExitRuntime, "DISARM", err.Error(), "")
	}
	return cc.Result(
		struct {
			Engine string `json:"engine"`
			Level  int    `json:"level"`
		}{c.Engine, c.Level},
		func() { fmt.Fprintf(cc.Stdout, "XWBDEBUG restored to level %d on %s\n", c.Level, c.Engine) },
	)
}
