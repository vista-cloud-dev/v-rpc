---
title: v-rpc implementation plan — the `v rpc debug` RPC-tap viewer/capture
status: draft
version: v0.1.0
created: 2026-06-26
last_modified: 2026-06-26
doc_type: [PLAN]
layer: v
---

# v-rpc — `v rpc debug` (RPC Broker debug tap: view + save)

## Purpose

A **debug/validation** tool. `v rpc debug` taps the RPC Broker's *native*
`XWBDEBUG` log (`^XTMP("XWBLOG"_$J)`) over the m engine seam to **view live RPC
traffic in the terminal** and **save it to a file** for **offline comparison
against the Phase-2 VSL tap**. Goals: validate the VSL tap captures correctly,
troubleshoot capture, and troubleshoot CPRS RPC generation. Correlation of the
XWBDEBUG capture with the S3 tap is **offline and separate** — this tool only
*produces comparable output* (LDJSON whose field names align with the s3tap
envelope: `rpc`, `ts`, `job`, `seq`).

XWBDEBUG is the zero-install **oracle**; the durable egress-to-S3 tap is the VSL
hook (separate work). See `docs/proposals/considering/cprs-rpc-xwbdebug-smoke-test.md`
and the `cprs-rpc-xwbdebug-host-probe` memory.

## Design (locked 2026-06-26 with owner)

| Decision | Choice |
|---|---|
| Repo | `v-rpc` (new), exports importable `rpccli`; `v` umbrella mounts `v rpc` |
| Command group | `v rpc debug …` (scoped — `v rpc` will carry other verbs later) |
| Verbs | `tail` (live CLI viewer), `capture --out file://…` (LDJSON), `status`, `arm`/`disarm` |
| Viewer | CLI now; structured so a TUI drops in later |
| Sinks | terminal + local file (LDJSON). **No S3 here** (offline correlation is separate) |
| Engine | explicit `--engine ydb\|iris` (required) + driver transport; container via `M_<ENGINE>_*` env |
| Seam | `mdriver.Client` only (waterline rule 3); clikit (kong); Go template furniture |
| GitHub remote | owner's step (`gh repo create`); v-cli mount deferred until published + tagged |

## Architecture (waterline-clean)

- `internal/xwblog` — **pure** parse/record/LDJSON/dedup. No engine dep. ✅ done
- `internal/xwbwire` — **pure** [XWB] broker wire-message encoder (for `ping`). ✅ done
- `internal/capture` — arm/disarm + poll-read + dedup + render/emit, over a small
  `Execer` interface (fake-tested; the real impl wraps `mdriver.Client.ExecEval`).
- `rpccli` — clikit command structs (`Commands`, `debugCmd` + subcommands),
  `engineConn`-style flags, adapts `mdriver.Client` to `capture.Execer`.
- `main.go` — thin standalone binary via `clikit.Run`.

## Increment tracker

- [x] **I0 — scaffold**: furniture from v-pkg (Makefile/golangci/CI/license),
  `go.mod`, `repo.meta.json` (layer `v`). 
- [x] **I1 — `internal/xwblog`** (TDD green): `ParseRecord`, `Kind` classify,
  `Key()` (per-$J-wipe race guard), `LDJSON()` (s3tap-aligned), `HHMMSS`.
- [x] **I2 — `internal/capture`** (TDD green, fake Execer): `Arm/Disarm` (XPAR
  level + read-back confirm), `ReadAll`/`Tailer.ReadNew` (poll + dedup), `Clear`,
  `Level`, marker-based reader parse (newline-encoding tolerant).
- [x] **I3b — `v rpc debug ping`** (TDD `xwbwire` 100%): fires no-arg [XWB] RPCs
  at a broker (`--addr`) so capture has self-contained traffic — no python/CPRS.
- [x] **I3 — `rpccli`**: `v rpc debug {tail,capture,status,arm,disarm,ping}` with
  `--all/--filter/--interval/--duration/--level/--keep/--no-clear`, engine flags,
  real `mdriver.Client` adapter; `main.go`. `make check` green (gofmt+lint+race+build).
  **Live `status` proven** through the real driver against vehu (level 1, as-found).
  Live `arm`/`capture` streaming validation deferred to owner (state-changing ops).
- [x] **I4 — README + user guide + memory**; local commits. Owner `gh repo create` still owed.
- [x] **Live validation against real CPRS (2026-06-26)** — captured a full sign-on
  (1,120 RPCs / 242 distinct / 7 connections) to `cprs-login.ldjson`; canonical
  signon sequence verified. vehu login via documented `worldvista/vehu` creds
  (PROVIDER,VERO `CAS123`), CPRS→broker via the socat relay. Capture `*.ldjson`
  gitignored. Known interaction: restore-to-found-level — `disarm` forces back to 1.
- [ ] **I5 (deferred)** — `vcontract.Contract()` + mount into `v-cli` (needs the
  published, tagged repo; v-cli pins versions, no `replace`).

- [x] **Minimal config / portability (2026-06-26)** — engine flags read env
  (`VRPC_*`); `--engine` defaults to `ydb`; driver auto-located next to `v-rpc` on
  PATH (no `M_<ENGINE>_BIN`); `make install`. Net: only `VRPC_CONTAINER` needed,
  `v-rpc debug status` runs fully flagless. Verified against vehu.

## Non-goals (this repo)

S3/MinIO egress; request↔response correlation; payloads/results (XWBDEBUG has
none — that is VSL-hook/s3tap territory); the offline join itself.
