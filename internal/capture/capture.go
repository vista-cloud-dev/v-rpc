// Package capture drives the RPC Broker's XWBDEBUG log over the m engine seam:
// arm/disarm the debug level, poll-read ^XTMP("XWBLOG"*), and dedup into a live
// tail. It depends only on a tiny Execer interface (one M one-liner in, captured
// device output out), so it is fully unit-testable; the real Execer wraps
// mdriver.Client.ExecEval in the rpccli layer (the single transport seam).
package capture

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/vista-cloud-dev/v-rpc-debug/internal/xwblog"
)

// Execer runs a single M command against a live engine and returns its captured
// device output. The only allowed implementation reaches the engine through
// mdriver.Client (waterline rule 3).
type Execer interface {
	Exec(ctx context.Context, command string) (string, error)
}

// Record is re-exported for callers so they need not import xwblog directly.
type Record = xwblog.Record

// The XPAR entity/parameter for the broker's debug level (FileMan #8989.5).
const debugParam = `"SYS","XWBDEBUG"`

// M one-liners. Results are wrapped in <<R>>..<<E>> markers so they survive the
// driver's device-output capture regardless of newline encoding. The reader
// emits one TAB-delimited job<TAB>seq<TAB>value record per node, skipping the 0
// banner and .1 counter ($ORDER from .1 yields 1,2,3…).
const (
	readerM = `S J="XWBLOG" F  S J=$O(^XTMP(J)) Q:$E(J,1,6)'="XWBLOG"  S N=.1 F  S N=$O(^XTMP(J,N)) Q:N=""  W "<<R>>",J,$C(9),N,$C(9),^XTMP(J,N),"<<E>>",$C(10)`
	levelM  = `W "<<R>>",$$GET^XPAR(` + debugParam + `),"<<E>>"`
	clearM  = `S X="XWBLOG" F  S X=$O(^XTMP(X)) Q:$E(X,1,6)'="XWBLOG"  K ^XTMP(X)`
)

func setLevelM(level int) string {
	return fmt.Sprintf(`S DUZ=1,DUZ(0)="@",U="^",DT=$$DT^XLFDT D EN^XPAR(`+debugParam+`,1,%d)`, level)
}

var markerRE = regexp.MustCompile(`(?s)<<R>>(.*?)<<E>>`)

// ReadAll runs the XWBLOG reader and returns every logged line, in engine
// $ORDER (job ascending, seq ascending).
func ReadAll(ctx context.Context, ex Execer) ([]Record, error) {
	out, err := ex.Exec(ctx, readerM)
	if err != nil {
		return nil, err
	}
	var recs []Record
	for _, m := range markerRE.FindAllStringSubmatch(out, -1) {
		job, rest, ok := strings.Cut(m[1], "\t")
		if !ok {
			continue
		}
		seqStr, value, ok := strings.Cut(rest, "\t")
		if !ok {
			continue
		}
		seq, err := strconv.Atoi(seqStr)
		if err != nil {
			continue
		}
		recs = append(recs, xwblog.ParseRecord(job, seq, value))
	}
	return recs, nil
}

// Arm sets SYS XWBDEBUG to level and confirms the read-back matches.
func Arm(ctx context.Context, ex Execer, level int) error {
	if _, err := ex.Exec(ctx, setLevelM(level)); err != nil {
		return err
	}
	got, err := Level(ctx, ex)
	if err != nil {
		return err
	}
	if got != level {
		return fmt.Errorf("XWBDEBUG read back as %d, expected %d", got, level)
	}
	return nil
}

// Disarm restores SYS XWBDEBUG to level (use 1 to leave a stock engine as found).
func Disarm(ctx context.Context, ex Execer, level int) error {
	return Arm(ctx, ex, level)
}

// Level reads the current SYS XWBDEBUG value.
func Level(ctx context.Context, ex Execer) (int, error) {
	out, err := ex.Exec(ctx, levelM)
	if err != nil {
		return 0, err
	}
	m := markerRE.FindStringSubmatch(out)
	if m == nil {
		return 0, fmt.Errorf("no level in engine output: %q", out)
	}
	v := strings.TrimSpace(m[1])
	if v == "" {
		return 0, nil // unset parameter reads as empty
	}
	return strconv.Atoi(v)
}

// Clear kills all ^XTMP("XWBLOG"*) nodes (a clean capture slate).
func Clear(ctx context.Context, ex Execer) error {
	_, err := ex.Exec(ctx, clearM)
	return err
}

// Tailer tracks which records have been emitted so each poll yields only new
// lines. The dedup key (xwblog.Record.Key) survives per-$J LOGSTART wipes.
type Tailer struct{ seen map[string]struct{} }

// NewTailer returns an empty Tailer.
func NewTailer() *Tailer { return &Tailer{seen: map[string]struct{}{}} }

// ReadNew reads the log and returns only records not seen before, in $ORDER.
func (t *Tailer) ReadNew(ctx context.Context, ex Execer) ([]Record, error) {
	all, err := ReadAll(ctx, ex)
	if err != nil {
		return nil, err
	}
	var fresh []Record
	for _, r := range all {
		k := r.Key()
		if _, ok := t.seen[k]; ok {
			continue
		}
		t.seen[k] = struct{}{}
		fresh = append(fresh, r)
	}
	return fresh, nil
}
