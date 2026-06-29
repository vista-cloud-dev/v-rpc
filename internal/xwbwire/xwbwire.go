// Package xwbwire encodes RPC Broker "[XWB]" wire-protocol messages — just
// enough to fire a no-argument RPC at a broker so a debug tap has traffic to
// capture. It is pure byte assembly (no I/O), so it is unit-testable; the TCP
// send lives in the rpccli ping command.
//
// This is the v rpc-debug domain acting as an RPC *client* (like CPRS), talking the
// broker's TCP wire protocol — distinct from reaching the M engine, which always
// goes through the m-driver-sdk seam.
//
// Message (client→server): "[XWB]" + 4 header bytes + chunks + 0x04 (EOT).
// Header "0030" = VER,TYPE,LENV,RT as ASCII digits (the minimal header). A chunk
// is a 1-byte type then its body; type '2' is an RPC chunk = SREAD(ver) +
// SREAD(name). SREAD = one length byte (≤255) + value. The broker logs
// "RPC: <name>" at parse, before any session check, so an unauthenticated ping is
// enough to exercise capture (it is then rejected — harmless).
package xwbwire

// sread encodes a value as one length byte followed by the bytes (len ≤ 255).
func sread(s string) []byte {
	b := []byte(s)
	return append([]byte{byte(len(b))}, b...)
}

// RPCMessage builds the wire bytes for a no-argument RPC named name.
func RPCMessage(name string) []byte {
	msg := []byte("[XWB]0030") // marker + minimal header
	msg = append(msg, '2')     // chunk type 2 = RPC
	msg = append(msg, sread("0")...)
	msg = append(msg, sread(name)...)
	msg = append(msg, 0x04) // EOT
	return msg
}
