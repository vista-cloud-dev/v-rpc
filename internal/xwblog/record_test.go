package xwblog

import "testing"

func TestParseRecord(t *testing.T) {
	tests := []struct {
		name    string
		job     string
		seq     int
		value   string
		wantPID string
		wantHT  string
		wantMsg string
		wantK   Kind
		wantRPC string
	}{
		{
			name: "rpc line", job: "XWBLOG416", seq: 4,
			value:   "67747,48090^RPC: XWB IM HERE",
			wantPID: "416", wantHT: "67747,48090", wantMsg: "RPC: XWB IM HERE",
			wantK: KindRPC, wantRPC: "XWB IM HERE",
		},
		{
			name: "log start", job: "XWBLOG416", seq: 1,
			value:   "67747,48090^Log start: Jun 26, 2026@13:21:30",
			wantPID: "416", wantHT: "67747,48090",
			wantMsg: "Log start: Jun 26, 2026@13:21:30", wantK: KindStart,
		},
		{
			name: "reject keeps caret in message", job: "XWBLOG416", seq: 5,
			value:   "67747,48090^reject: ^172.17.0.1",
			wantPID: "416", wantHT: "67747,48090",
			wantMsg: "reject: ^172.17.0.1", wantK: KindReject,
		},
		{
			name: "rpc name with trailing spaces trimmed", job: "XWBLOG7", seq: 2,
			value:   "67747,1^RPC:  XUS INTRO MSG ",
			wantPID: "7", wantHT: "67747,1", wantMsg: "RPC:  XUS INTRO MSG ",
			wantK: KindRPC, wantRPC: "XUS INTRO MSG",
		},
		{
			name: "tcpm connection line", job: "XWBLOG9", seq: 2,
			value:   "67747,1^XWBTCPM",
			wantPID: "9", wantHT: "67747,1", wantMsg: "XWBTCPM", wantK: KindConn,
		},
		{
			name: "no caret degrades gracefully", job: "XWBLOG9", seq: 9,
			value:   "weird",
			wantPID: "9", wantHT: "", wantMsg: "weird", wantK: KindOther,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseRecord(tt.job, tt.seq, tt.value)
			if got.PID != tt.wantPID {
				t.Errorf("PID = %q, want %q", got.PID, tt.wantPID)
			}
			if got.HTime != tt.wantHT {
				t.Errorf("HTime = %q, want %q", got.HTime, tt.wantHT)
			}
			if got.Message != tt.wantMsg {
				t.Errorf("Message = %q, want %q", got.Message, tt.wantMsg)
			}
			if got.Kind != tt.wantK {
				t.Errorf("Kind = %q, want %q", got.Kind, tt.wantK)
			}
			if got.RPC != tt.wantRPC {
				t.Errorf("RPC = %q, want %q", got.RPC, tt.wantRPC)
			}
			if got.Seq != tt.seq {
				t.Errorf("Seq = %d, want %d", got.Seq, tt.seq)
			}
			if got.Job != tt.job {
				t.Errorf("Job = %q, want %q", got.Job, tt.job)
			}
		})
	}
}

func TestRecordKeyDistinguishesConnections(t *testing.T) {
	// Same job + seq but different value (a recycled PID's new connection) must
	// produce different keys, or the tailer would drop the new connection's
	// lines as already-seen. This is the per-$J wipe race guard.
	a := ParseRecord("XWBLOG416", 1, "67747,100^Log start: A")
	b := ParseRecord("XWBLOG416", 1, "67747,205^Log start: B")
	if a.Key() == b.Key() {
		t.Fatalf("keys collide across connections: %q", a.Key())
	}
	// Identical line is identical key (idempotent dedup).
	c := ParseRecord("XWBLOG416", 1, "67747,100^Log start: A")
	if a.Key() != c.Key() {
		t.Errorf("identical records have different keys: %q vs %q", a.Key(), c.Key())
	}
}

func TestHHMMSS(t *testing.T) {
	tests := map[string]string{
		"67747,48090": "13:21:30",
		"67747,0":     "00:00:00",
		"1,86399":     "23:59:59",
		"":            "--:--:--",
		"garbage":     "--:--:--",
	}
	for in, want := range tests {
		if got := HHMMSS(in); got != want {
			t.Errorf("HHMMSS(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLDJSON(t *testing.T) {
	r := ParseRecord("XWBLOG416", 4, "67747,48090^RPC: XWB IM HERE")
	got := r.LDJSON()
	// Field names aligned with the s3tap envelope (rpc, ts, job, seq) so the two
	// captures can be joined offline; source marks the provenance.
	for _, want := range []string{
		`"source":"xwbdebug"`,
		`"rpc":"XWB IM HERE"`,
		`"ts":"67747,48090"`,
		`"job":416`,
		`"seq":4`,
		`"kind":"rpc"`,
	} {
		if !contains(got, want) {
			t.Errorf("LDJSON missing %s\n got: %s", want, got)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
