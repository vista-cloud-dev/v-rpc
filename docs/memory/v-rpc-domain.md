---
name: v-rpc-domain
description: v-rpc-debug is a new Go `v` domain (`v rpc-debug`) that taps the RPC Broker's native XWBDEBUG log over the m-driver-sdk seam to view/save live RPC traffic for offline comparison against the VSL tap. Built 2026-06-26; v-cli mount deferred until the repo is published+tagged.
metadata:
  type: project
---

**v-rpc-debug created 2026-06-26.** A new repo under vista-cloud-dev: the `v rpc`
domain, Go, exports importable `rpccli` (the `v` umbrella will mount it as
`v rpc`, mirroring v-pkg/pkgcli). Layer `v`. Headline capability **`v rpc-debug`**
taps the RPC Broker's *native* `XWBDEBUG` log (`^XTMP("XWBLOG"_$J)`) over the m
engine driver seam to **view live RPC traffic in the terminal** or **save it to a
file as LDJSON** for **offline comparison against the Phase-2 VSL tap** — a
debug/validation tool, NOT a durable egress tap (that's the VSL hook).

**Locked design (with owner):** `v rpc-debug {tail,capture,status,arm,disarm,clear,ping}`
(`ping` fires no-arg [XWB] RPCs at a broker to self-test capture — RPC-client role,
takes `--addr`, not the engine seam);
shared flags `--all/--filter/--interval/--duration/--level{2,3}/--keep/--no-clear`;
`--engine` defaults to `ydb` (IRIS = explicit opt-in for VA validation);
capture LDJSON field names align with the s3tap envelope (`rpc`,`ts`,`job`,`seq`)
so the two captures can be **joined offline and separately** — correlation is NOT
in this tool. Level 3 logs params = PHI (default 2 = names only). CLI viewer now;
TUI later. Engine flags also read env (`VRPC_ENGINE`/`VRPC_TRANSPORT`/
`VRPC_CONTAINER`, `VRPC_ADDR` for ping) via kong `env:""` tags — set once (direnv
`.envrc` / shell rc) and omit the flags; a CLI flag overrides its env var.

**MINIMAL CONFIG (2026-06-26):** driver auto-located by `mdriver.Locate` when the
`m-ydb`/`m-iris` binary sits next to `v-rpc-debug` on PATH (rule: $M_<ENGINE>_BIN → exe-dir
→ sibling dist/ → PATH) — so co-install both via `make install BINDIR=…` + drop the
driver in the same dir, and **no `M_<ENGINE>_BIN` needed**. With engine defaulting to
ydb + transport docker, the ONLY irreducible config is the container
(`VRPC_CONTAINER`); `v-rpc-debug status` then runs fully flagless. Verified.

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

**RESTORE/CLEAR (added 2026-06-26):** `tail`/`capture` restore XWBDEBUG to the
level they *found* at start, so overlapping runs can leave it armed — pass
`--restore-to 1` to force stock on exit, or `v rpc-debug disarm`. `v rpc-debug
clear` wipes the buffered `^XTMP("XWBLOG"*)` on demand (it otherwise auto-purges in
~7 days).

**OWED (owner):**
1. `gh repo create vista-cloud-dev/v-rpc-debug` + push `main` (repo creation is the
   owner's step per org convention).
2. ✅ DONE 2026-06-26 — live-validated against real CPRS (see VALIDATED above).
3. ✅ **I5 DONE 2026-06-26: mounted into v-cli.** Added `rpccli.Contract()`
   (`rpccli/contract.go`, mirrors pkgcli/contract.go; `Version="0.1.0"`,
   `ContractVersion="1.0"`, imports `v-pkg/vcontract` for the shared `Manifest`
   type — a v→v dep, waterline-OK). Repinned clikit v0.4.0 + added the shared
   discovery surface to the standalone (`explore`/`schema`/`version` under
   Introspect, `debug` under a Capture group). Tagged **v-rpc-debug v0.1.0**; v-cli now
   has `Rpc rpccli.Commands` (group Domains) + `rpccli.Contract()` in
   `buildRegistry()` (golden `dist/v-registry.json` regenerated). `v rpc <verb>`
   live on PATH. **GOTCHA:** fetching a fresh private-repo tag needs
   `go env -w GOPRIVATE=github.com/vista-cloud-dev` + `gh auth setup-git` — the
   default `proxy.golang.org,direct` can't auth and fails with "could not read
   Username for github.com".
