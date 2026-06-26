// Package xwblog parses the RPC Broker's native XWBDEBUG debug log
// (^XTMP("XWBLOG"_$J)) into structured records. It is pure logic with no engine
// or transport dependency, so the whole tap pipeline is unit-testable; the
// driver-facing read/arm/poll lives one layer up (internal/capture).
//
// A stored log node value is "$H^message" (the line's $HOROLOG timestamp, a
// caret, then up to 255 chars of text — see XWBDLOG LOG^). The $H prefix has no
// caret, so the first caret cleanly splits timestamp from message even when the
// message itself contains carets (e.g. "reject: ^172.17.0.1").
package xwblog

import (
	"strconv"
	"strings"
)

// Kind classifies an XWBLOG line by its role in a broker handler's lifecycle.
type Kind string

const (
	KindRPC    Kind = "rpc"    // "RPC: <name>" — the line that proves traffic
	KindStart  Kind = "start"  // "Log start: ..."
	KindConn   Kind = "conn"   // "XWBTCPM" / "MSG format ..."
	KindReject Kind = "reject" // "reject: ^<ip>" — no-session rejection
	KindOther  Kind = "other"
)

// Record is one parsed XWBLOG line.
type Record struct {
	Job     string // log subscript, e.g. "XWBLOG416"
	PID     string // numeric tail of Job, e.g. "416"
	Seq     int    // node number N under the job
	HTime   string // $HOROLOG prefix, e.g. "67747,48090"
	Message string // the text after the $H caret
	Kind    Kind
	RPC     string // RPC name when Kind == KindRPC, else ""
}

// ParseRecord splits a stored node value ("$H^message") into a Record.
func ParseRecord(job string, seq int, value string) Record {
	r := Record{Job: job, Seq: seq, PID: strings.TrimPrefix(job, "XWBLOG")}
	if ht, msg, ok := strings.Cut(value, "^"); ok {
		r.HTime, r.Message = ht, msg
	} else {
		r.Message = value
	}
	r.Kind, r.RPC = classify(r.Message)
	return r
}

func classify(msg string) (Kind, string) {
	switch {
	case strings.HasPrefix(msg, "RPC:"):
		return KindRPC, strings.TrimSpace(strings.TrimPrefix(msg, "RPC:"))
	case strings.HasPrefix(msg, "Log start:"):
		return KindStart, ""
	case strings.HasPrefix(msg, "reject:"):
		return KindReject, ""
	case msg == "XWBTCPM" || strings.HasPrefix(msg, "MSG format"):
		return KindConn, ""
	default:
		return KindOther, ""
	}
}

// Key is the dedup identity of a record for the tailer. It must distinguish a
// recycled PID's new connection from the old one, so it includes the raw
// timestamp+message (a new connection's lines carry a fresh $H), not just
// job+seq which collide after a LOGSTART wipe reuses the same PID.
func (r Record) Key() string {
	return r.Job + "\x01" + strconv.Itoa(r.Seq) + "\x01" + r.HTime + "\x01" + r.Message
}

// LDJSON renders the record as one JSON object (no trailing newline), with field
// names aligned to the s3tap envelope (rpc, ts, job, seq) for offline comparison.
func (r Record) LDJSON() string {
	var b strings.Builder
	b.WriteString(`{"source":"xwbdebug","schema_version":1`)
	b.WriteString(`,"kind":` + quote(string(r.Kind)))
	if r.RPC != "" {
		b.WriteString(`,"rpc":` + quote(r.RPC))
	}
	b.WriteString(`,"ts":` + quote(r.HTime))
	if job, err := strconv.Atoi(r.PID); err == nil {
		b.WriteString(`,"job":` + strconv.Itoa(job))
	} else {
		b.WriteString(`,"job":` + quote(r.PID))
	}
	b.WriteString(`,"seq":` + strconv.Itoa(r.Seq))
	b.WriteString(`,"msg":` + quote(r.Message))
	b.WriteString("}")
	return b.String()
}

// quote JSON-encodes a string with surrounding quotes, escaping the characters
// that can appear in broker log text (quotes, backslash, control chars).
func quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				b.WriteString(`\u00`)
				const hex = "0123456789abcdef"
				b.WriteByte(hex[r>>4])
				b.WriteByte(hex[r&0xf])
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// HHMMSS converts a $HOROLOG "days,secs" to a clock string in engine-local time,
// or "--:--:--" if it cannot be parsed.
func HHMMSS(htime string) string {
	_, secStr, ok := strings.Cut(htime, ",")
	if !ok {
		return "--:--:--"
	}
	secs, err := strconv.Atoi(secStr)
	if err != nil || secs < 0 || secs > 86399 {
		return "--:--:--"
	}
	h, m, s := secs/3600, (secs%3600)/60, secs%60
	return pad2(h) + ":" + pad2(m) + ":" + pad2(s)
}

func pad2(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
