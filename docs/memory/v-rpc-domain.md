---
name: v-rpc-domain
description: v-rpc is a new Go `v` domain (`v rpc debug`) that taps the RPC Broker's native XWBDEBUG log over the m-driver-sdk seam to view/save live RPC traffic for offline comparison against the VSL tap. Built 2026-06-26; v-cli mount deferred until the repo is published+tagged.
metadata:
  type: project
---

**v-rpc created 2026-06-26.** A new repo under vista-cloud-dev: the `v rpc`
domain, Go, exports importable `rpccli` (the `v` umbrella will mount it as
`v rpc`, mirroring v-pkg/pkgcli). Layer `v`. Headline capability **`v rpc debug`**
taps the RPC Broker's *native* `XWBDEBUG` log (`^XTMP("XWBLOG"_$J)`) over the m
engine driver seam to **view live RPC traffic in the terminal** or **save it to a
file as LDJSON** for **offline comparison against the Phase-2 VSL tap** — a
debug/validation tool, NOT a durable egress tap (that's the VSL hook).

**Locked design (with owner):** `v rpc debug {tail,capture,status,arm,disarm,ping}`
(`ping` fires no-arg [XWB] RPCs at a broker to self-test capture — RPC-client role,
takes `--addr`, not the engine seam);
shared flags `--all/--filter/--interval/--duration/--level{2,3}/--keep/--no-clear`;
explicit `--engine ydb|iris` (ydb/vehu now, IRIS-VistA for VA validation later);
capture LDJSON field names align with the s3tap envelope (`rpc`,`ts`,`job`,`seq`)
so the two captures can be **joined offline and separately** — correlation is NOT
in this tool. Level 3 logs params = PHI (default 2 = names only). CLI viewer now;
TUI later.

**Architecture (waterline-clean):** `internal/xwblog` = pure parse/record/LDJSON/
dedup (no engine dep, TDD); `internal/capture` = arm/disarm + poll + dedup over a
small `Execer` interface (fake-tested); `rpccli` = clikit (kong) command surface
adapting `mdriver.Client` to `Execer`; `main.go` = standalone binary. Engine
access ONLY via `mdriver.Client` (waterline rule 3), never raw `docker exec`. The
per-$J LOGSTART wipe makes XWBLOG lossy for complete capture — documented; fine
for the oracle role. See the vehu-side mechanics in the shared
`cprs-rpc-xwbdebug-host-probe` memory.

**State:** `make check` green (gofmt+golangci-lint+race+build); `internal/*`
74–80% covered. Deps pin clikit v0.1.0 + m-driver-sdk v0.3.0 (airgapped, no
`replace`). Furniture from v-pkg/go-cli-template.

**VALIDATED END-TO-END against real CPRS (2026-06-26).** Live `tail`/`capture` +
`ping` all exercised against vehu through the real driver. Captured a complete
CPRS sign-on to `cprs-login.ldjson`: 1,120 RPC records, 242 distinct RPCs, 7
broker connections. Canonical signon verified — `XUS SIGNON SETUP` → `XUS INTRO
MSG` → `XUS AV CODE` → `XUS GET USER INFO` → `XWB GET BROKER INFO` → `XUS DIVISION
GET` → `XWB CREATE CONTEXT`×4 → chart-load RPCs (ORWDX/ORWU/TIU/ORQQ…). vehu login
via documented `worldvista/vehu` Docker Hub creds (PROVIDER,VERO access `CAS123`;
access codes confirmed read-only via `$$EN^XUSHSH` + #200 "A" index — no mutation).
CPRS-in-VBox reaches the loopback broker via the `socat` relay
([[vehu-broker-vbox-relay]], CPRS → `10.0.2.2:19431`). Capture `*.ldjson` is
gitignored (data, not source).

**KNOWN INTERACTION:** `tail`/`capture` restore XWBDEBUG to the level they *found*
at start. Overlapping runs (capture started while a tail already armed level 2)
leave it at 2, not 1 — run `v rpc debug disarm` to force back to 1. (There is no
standalone `clear` verb yet — buffered XWBLOG auto-purges in ~7 days.)

**OWED (owner):**
1. `gh repo create vista-cloud-dev/v-rpc` + push `main` (repo creation is the
   owner's step per org convention).
2. ✅ DONE 2026-06-26 — live-validated against real CPRS (see VALIDATED above).
3. **I5 (deferred): mount into v-cli** — add `vcontract.Contract()` to `rpccli`
   (mirror pkgcli/contract.go), then in v-cli add `Rpc rpccli.Commands` +
   `rpccli.Contract()` to the registry. Needs v-rpc published + tagged (v-cli pins
   versions, no `replace`).
