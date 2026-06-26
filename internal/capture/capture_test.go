package capture

import (
	"context"
	"strings"
	"testing"
)

// fakeExecer returns canned stdout per call and records the commands it saw.
type fakeExecer struct {
	out  []string // queued responses, consumed in order; last one repeats
	cmds []string
	err  error
}

func (f *fakeExecer) Exec(_ context.Context, command string) (string, error) {
	f.cmds = append(f.cmds, command)
	if f.err != nil {
		return "", f.err
	}
	if len(f.out) == 0 {
		return "", nil
	}
	o := f.out[0]
	if len(f.out) > 1 {
		f.out = f.out[1:]
	}
	return o, nil
}

// reader output uses TAB-delimited job\tseq\tvalue between <<R>>..<<E>> markers.
const twoConns = "<<R>>XWBLOG416\t1\t67747,48090^Log start: Jun 26, 2026@13:21:30<<E>>\n" +
	"<<R>>XWBLOG416\t4\t67747,48090^RPC: XWB IM HERE<<E>>\n" +
	"<<R>>XWBLOG423\t4\t67747,48090^RPC: XUS INTRO MSG<<E>>\n"

func TestReadAll(t *testing.T) {
	ex := &fakeExecer{out: []string{twoConns}}
	recs, err := ReadAll(context.Background(), ex)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("got %d records, want 3", len(recs))
	}
	if recs[1].RPC != "XWB IM HERE" {
		t.Errorf("recs[1].RPC = %q", recs[1].RPC)
	}
	if recs[2].PID != "423" {
		t.Errorf("recs[2].PID = %q", recs[2].PID)
	}
	if !strings.Contains(ex.cmds[0], "XWBLOG") {
		t.Errorf("reader cmd did not query XWBLOG: %q", ex.cmds[0])
	}
}

func TestReadAllToleratesRealNewlinesInBlob(t *testing.T) {
	// Whether the driver returns marker-separated records joined by real or
	// escaped newlines, extraction must work (dotall over markers).
	ex := &fakeExecer{out: []string{strings.ReplaceAll(twoConns, "\n", "")}}
	recs, err := ReadAll(context.Background(), ex)
	if err != nil || len(recs) != 3 {
		t.Fatalf("got %d recs, err %v; want 3", len(recs), err)
	}
}

func TestTailerReadNewDedups(t *testing.T) {
	grown := twoConns + "<<R>>XWBLOG999\t4\t67747,48099^RPC: ORWU USERINFO<<E>>\n"
	ex := &fakeExecer{out: []string{twoConns, twoConns, grown}}
	tl := NewTailer()

	first, _ := tl.ReadNew(context.Background(), ex)
	if len(first) != 3 {
		t.Fatalf("first poll got %d, want 3", len(first))
	}
	second, _ := tl.ReadNew(context.Background(), ex)
	if len(second) != 0 {
		t.Fatalf("second poll got %d, want 0 (all seen)", len(second))
	}
	third, _ := tl.ReadNew(context.Background(), ex)
	if len(third) != 1 || third[0].RPC != "ORWU USERINFO" {
		t.Fatalf("third poll got %v, want just ORWU USERINFO", third)
	}
}

func TestArmConfirmsLevel(t *testing.T) {
	// Arm sets the param then reads it back; a matching level succeeds.
	ex := &fakeExecer{out: []string{"", "<<R>>2<<E>>"}}
	if err := Arm(context.Background(), ex, 2); err != nil {
		t.Fatalf("Arm: %v", err)
	}
	if !strings.Contains(ex.cmds[0], "EN^XPAR") || !strings.Contains(ex.cmds[0], "XWBDEBUG") {
		t.Errorf("arm cmd not an XPAR set: %q", ex.cmds[0])
	}
}

func TestArmRejectsMismatch(t *testing.T) {
	ex := &fakeExecer{out: []string{"", "<<R>>1<<E>>"}} // set 2 but reads 1
	if err := Arm(context.Background(), ex, 2); err == nil {
		t.Fatal("Arm should error when read-back level != requested")
	}
}

func TestLevel(t *testing.T) {
	ex := &fakeExecer{out: []string{"<<R>>2<<E>>"}}
	lvl, err := Level(context.Background(), ex)
	if err != nil || lvl != 2 {
		t.Fatalf("Level = %d, %v; want 2, nil", lvl, err)
	}
}
