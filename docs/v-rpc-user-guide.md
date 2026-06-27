---
title: v-rpc user guide — viewing and saving live RPC traffic with `v rpc debug`
status: draft
version: v0.1.0
created: 2026-06-26
last_modified: 2026-06-27
doc_type: [GUIDE]
layer: v
---

# v-rpc user guide — `v rpc debug`

`v rpc debug` shows you the RPCs flowing through a VistA RPC Broker, live in your
terminal, and can save them to a file for later analysis. It does this with the
Broker's **own** debug facility (`XWBDEBUG`) — nothing is installed on the engine
and nothing is patched; turning it on is a single parameter toggle that the tool
manages for you.

Use it to answer questions like:

- *Is CPRS (or any client) actually reaching the broker, and what is it calling?*
- *Did my Phase-2 VSL tap capture the same RPCs the broker really saw?* (offline
  comparison — see [Comparing against the VSL tap](#7-comparing-against-the-vsl-tap))
- *Why is this client misbehaving — which RPC fires, in what order?*

> **What it is not.** This is a **debug/validation** tool, not a durable capture
> pipeline. `XWBDEBUG` logs RPC **names** (not results, not request↔response
> correlation), it can drop traffic under load (see [Limitations](#8-limitations)),
> and it has no S3/egress. The durable, complete, egress-capable tap is the VSL
> broker hook; `v rpc debug` is the zero-install **oracle** you check it against.

## Contents

1. [Setup (one time)](#1-setup-one-time)
2. [Selecting the engine](#2-selecting-the-engine)
3. [Logging into vehu — credentials, user, and the richest patient](#3-logging-into-vehu)
4. [The commands](#4-the-commands)
5. [Common flags](#5-common-flags)
6. [End-to-end run (copy/paste)](#6-end-to-end-run)
7. [Comparing against the VSL tap](#7-comparing-against-the-vsl-tap)
8. [Limitations](#8-limitations)
9. [Troubleshooting](#9-troubleshooting)
10. [References](#references)

---

## 1. Setup (one time)

`v-rpc` is a single static binary. It reaches the engine **only** through the
m-driver-sdk seam, so it needs the `m-<engine>` driver binary reachable — the one
external dependency. Put both binaries on your `PATH` **in the same directory** and
`v-rpc` locates the driver automatically (driver-contract §4) — **no `M_<ENGINE>_BIN`
to set**:

```bash
cd ~/vista-cloud-dev/v-rpc && make install BINDIR=~/scripts/bin   # installs v-rpc
install -m755 ~/vista-cloud-dev/m-ydb/dist/m-ydb ~/scripts/bin/   # co-locate the driver
```

The only thing `v-rpc` can't guess is **which engine container** to talk to. Set it
once — engine defaults to `ydb`, transport to `docker`:

```bash
export VRPC_CONTAINER=vehu        # once per shell, or in the repo .envrc (direnv)
```

That's the whole configuration. Everything is now flagless:

```bash
v-rpc debug status
v-rpc debug tail
v-rpc debug capture --out rpc.ldjson
```

**Requirements:** Docker running with the target engine container up (e.g. `vehu`).
For IRIS, co-locate the `m-iris` driver and add `--engine iris` (or
`VRPC_ENGINE=iris`). `M_<ENGINE>_BIN` still works as an explicit override if you
keep the driver somewhere else.

Once mounted into the `v` umbrella, every command below is also available as
`v rpc debug …`. Standalone, it is `v-rpc debug …` (the form used throughout).

## 2. Selecting the engine

Engine-side commands take three knobs, all with sensible defaults so you usually
override **nothing** (just set the container once, per §1):

| Flag | Default | Env | Meaning |
|---|---|---|---|
| `--engine` | `ydb` | `VRPC_ENGINE` | which engine: `ydb` or `iris` (IRIS = explicit opt-in for VA validation) |
| `--transport` | `docker` | `VRPC_TRANSPORT` | driver transport: `local`, `docker`, `remote` |
| `--container` | — | `VRPC_CONTAINER` | container/instance name; sets `M_<ENGINE>_CONTAINER` |

```bash
v-rpc debug status                                    # ydb / docker / $VRPC_CONTAINER
v-rpc debug status --engine iris --container foia-t12 # override ad hoc
```

(The connection — container, base URL, credentials — is otherwise read by the
driver from its `M_<ENGINE>_*` environment, exactly like `m vista`.)

### Setting defaults (skip the flags)

Every engine flag also reads an environment variable, so set them once and drop
the flags entirely:

| Flag | Env var |
|---|---|
| `--engine` | `VRPC_ENGINE` |
| `--transport` | `VRPC_TRANSPORT` |
| `--container` | `VRPC_CONTAINER` |
| `--addr` (`ping`) | `VRPC_ADDR` |

```bash
export M_YDB_BIN=~/vista-cloud-dev/m-ydb/dist/m-ydb     # driver location
export VRPC_ENGINE=ydb VRPC_TRANSPORT=docker VRPC_CONTAINER=vehu

v-rpc debug status        # no --engine/--transport/--container needed
v-rpc debug tail
v-rpc debug capture --out rpc.ldjson
```

A flag on the command line always **overrides** its env var, so you can keep ydb
as the default and switch to IRIS ad hoc with `--engine iris`.

**Make it persistent** with [direnv](https://direnv.net) — the house per-project
env backbone. Add to `~/vista-cloud-dev/v-rpc/.envrc` (then `direnv allow`):

```bash
export M_YDB_BIN="$HOME/vista-cloud-dev/m-ydb/dist/m-ydb"
export VRPC_ENGINE=ydb VRPC_TRANSPORT=docker VRPC_CONTAINER=vehu
```

so the defaults apply automatically whenever you `cd` into the repo. (Or put the
same `export`s in your shell rc for a machine-wide default.) The driver-native
`M_<ENGINE>_CONTAINER` (e.g. `M_YDB_CONTAINER=vehu`) also works for the container
if you prefer the driver's own variable.

## 3. Logging into vehu

To drive real traffic you log a client (CPRS) into VistA. For the `worldvista/vehu`
demo image these access/verify codes are documented and **confirmed against this
engine** (access-code hashes resolve to the named `#200` users — see
[References](#references)):

| User | Access | Verify | Role |
|---|---|---|---|
| **PROVIDER,VERO** | `CAS123` | `CAS123..` | **provider — recommended** |
| VEHU,TEN | `10VEHU` | `QXYG~011` | demo clinician |
| System Manager | `PRO1234` | `PRO1234!!` | admin / programmer |

E-signature code (to sign orders/notes): `123456`.

**Recommended user type → a clinical *provider*** (e.g. **PROVIDER,VERO** /
`CAS123`). A provider sees the full chart *and* can place orders, so a session
exercises the widest set of RPCs (orders, notes, consults, meds, labs, vitals) —
the richest traffic for tap validation. The System Manager account can browse but
isn't a clinician, so it fires a narrower set.

At the CPRS sign-on screen type the access code, Tab, then the verify code — or type
`CAS123;CAS123..` in the Access field. (On these demo images you may be prompted to
choose a new verify code on first login; just set one.)

### The patient with the most data

Picked from a live read-only probe of vehu (order/note/visit cross-references, see
[References](#references)). For the **richest, most varied** chart — the best choice
for exercising the most RPCs across tabs — open:

> **TEN,PATIENT** — **691 notes, 190 visits, 236 orders** (by far the most clinical
> documents and encounters of any patient). Search `TEN,PATIENT` in CPRS.

Runners-up with broad data: **FORTYSIX,PATIENT** (48 notes / 121 visits / 175
orders) and **THIRTYSIX,PATIENT** (64 / 68 / 170).

If you specifically want **maximum orders** (and nothing else), **SEVENTYEIGHT,
OUTPATIENT** has 1,508 orders — but **0 notes and 0 visits** (it's an order-load
patient), so it lights up far fewer tabs than TEN,PATIENT. For a representative
full-chart RPC sweep, prefer **TEN,PATIENT**.

## 4. The commands

### `status` — what's the broker doing?

Read-only. Shows the current `XWBDEBUG` level and how much is buffered.

```bash
v-rpc debug status --engine ydb --container vehu
# engine ydb: XWBDEBUG level 1 (off); 0 log job(s), 0 RPC line(s) buffered
```

### `tail` — watch RPCs live

Arms `XWBDEBUG`, clears the log for a clean slate, then streams new RPC lines to
your terminal until you press Ctrl-C. On exit it **restores the prior level**.

```bash
v-rpc debug tail --engine ydb --container vehu
# [13:21:30] job     416  RPC: XWB IM HERE
# [13:21:31] job     423  RPC: XUS INTRO MSG
```

Then drive some traffic (start CPRS, or fire test RPCs) and watch them appear.

### `capture` — save RPCs to a file

Same as `tail`, but appends each record to a file as **LDJSON** (one JSON object
per line). Use it to grab a sample for offline analysis.

```bash
v-rpc debug capture --engine ydb --container vehu --out rpc.ldjson --duration 30s
```

Each line looks like:

```json
{"source":"xwbdebug","schema_version":1,"kind":"rpc","rpc":"XWB IM HERE","ts":"67747,48090","job":416,"seq":4,"msg":"RPC: XWB IM HERE"}
```

By default it also echoes the captured RPCs to stderr so you can watch progress;
pass `--quiet` to suppress that.

### `ping` — fire test RPCs (drive traffic)

Sends harmless no-arg RPCs straight to the broker's TCP port so a running `tail`
or `capture` has something to capture — no external client (or CPRS) needed. The
broker logs each `RPC: <name>` then rejects it (no session); that's exactly what
we want to see in the tap.

```bash
v-rpc debug ping --addr 127.0.0.1:9430
#   sent XWB IM HERE               (broker replied 52 bytes)
#   sent XUS INTRO MSG             (broker replied 52 bytes)
#   sent XWB GET VARIABLE VALUE    (broker replied 52 bytes)
# 3/3 RPC(s) sent to 127.0.0.1:9430
```

`ping` takes a broker `--addr` (host:port — vehu is `127.0.0.1:9430`), **not** the
engine flags: unlike the other verbs it connects as an RPC *client* over the
broker wire protocol, it does not reach the M engine. Flags: `--rpc NAME`
(repeatable; default a small set), `--count N` (send the set N times),
`--timeout` (per-connection).

> If `XWBDEBUG` is **off**, a ping writes nothing (the broker's logger quits
> immediately) — so pinging is safe even when you're not capturing.

### `arm` / `disarm` — explicit control

`tail` and `capture` arm and restore automatically, so you usually don't need
these. Use them when you want `XWBDEBUG` left on across several separate steps
(e.g. arm, run a client by hand, then capture with `--no-clear`).

```bash
v-rpc debug arm    --engine ydb --container vehu          # set level 2 (names)
v-rpc debug disarm --engine ydb --container vehu          # restore to level 1 (stock)
```

### `clear` — wipe the buffered log

Kills all `^XTMP("XWBLOG"*)` nodes so the engine is pristine (the buffer otherwise
auto-purges after ~7 days). Reports how many lines it removed.

```bash
v-rpc debug clear --engine ydb --container vehu
# cleared 160 buffered XWBLOG line(s) on ydb
```

### Discovery & introspection (no engine needed)

A few top-level commands describe the tool itself — none of them touch an engine,
so they work with no driver, container, or Docker:

```bash
v-rpc menu             # browse the command surface interactively (palette)
v-rpc version          # show version and build info
v-rpc schema | jq .    # emit the command/flag/enum tree as JSON (agent/script discovery)
v-rpc install-completions   # install shell tab-completion
```

`menu` and `version` show in `--help`; `schema` and `install-completions` are
**hidden** from the help listing (machine/one-time use) but remain fully runnable.
`schema` is the machine-readable contract for the entire CLI (every command, flag,
default, and enum) — handy for scripting or for an agent discovering the surface.

#### `menu` — the interactive palette

`v-rpc menu` opens a full-screen, keyboard-driven palette over the tool's own
command tree — the fastest way to discover what's available without memorizing
flags. It draws a breadcrumb of where you are, the commands grouped by category
(**CAPTURE**, **COMMANDS**, …), and a one-line detail strip for whatever the
cursor is on: the full command path, a short summary, and a status badge —
`[runnable]` (green), `[needs args]` (blue), or `[group]` (gray, descends into
sub-commands).

| Key | Action |
|---|---|
| `←↑↓→` (or `h j k l`) | Move the cursor between categories and commands |
| `⏎` Enter | Descend into a group, or open the focused command (shows its help/flags) |
| `⌫` Backspace | Go back up one level |
| `/` | Fuzzy-filter the surface by typing; `⏎`/`Esc` ends filtering |
| `q` (or `Esc` / `Ctrl-C`) | Quit the palette |

So a typical browse is: `v-rpc menu` → arrow onto **debug** (a `[group]`) → `⏎`
to descend → land on `capture` to read its summary and badge → `⏎` to see its
flags. Nothing runs against the engine until you actually invoke a command on the
command line.

Run on a **non-interactive** stdout (no TTY — e.g. piped or redirected), `menu`
prints the full styled help instead of the live palette, so `v-rpc menu | less`
still gives you a readable overview.

## 5. Common flags

For `tail` and `capture`:

| Flag | Default | What it does |
|---|---|---|
| `--all` | off | Show every log line (connect/reject/etc.), not just `RPC:` lines |
| `--filter TEXT` | — | Only RPCs whose name contains `TEXT` (case-insensitive) |
| `--interval N` | `1.0` | Poll the log every `N` seconds |
| `--duration D` | `0` | Stop after `D` (e.g. `30s`, `5m`); `0` = run until Ctrl-C |
| `--level {2,3}` | `2` | Debug level to arm — **see the PHI warning below** |
| `--keep` | off | Leave `XWBDEBUG` armed on exit instead of restoring |
| `--no-clear` | off | Don't wipe the existing log on start (capture what's already buffered) |
| `--restore-to N` | `-1` | Level to restore on exit (`-1` = the level found at start). Use `1` to force stock if a prior/overlapping run left it armed |
| `--out PATH` | — | (`capture` only, required) output file; `file://PATH` or `PATH` |
| `--quiet` | off | (`capture` only) don't echo to the terminal |

### Output format & global flags

Every command honors the shared clikit globals:

| Flag | Meaning |
|---|---|
| `-o`, `--output {text,json,auto}` | `text` (styled, default on a TTY), `json` (machine-readable), or `auto` |
| `--no-color` | Disable ANSI styling even on a TTY |
| `-v`, `--verbose` | Verbose diagnostics to stderr |

For `tail`, `-o json` streams the same LDJSON records `capture` writes — handy for
piping into `jq`:

```bash
v-rpc debug tail --engine ydb --container vehu -o json | jq -r .rpc
```

### ⚠️ PHI warning — level 2 vs 3

- **Level 2 (default)** logs RPC **names only** — safe.
- **Level 3** logs the full call string **including RPC parameters**, which on a
  real VistA is **PHI**, written in cleartext to `^XTMP`. Only use `--level 3` on
  a test system with no real patient data, and clear the log afterward.

## 6. End-to-end run

### A. Self-contained (no CPRS) — ping the live broker, examine, clean up

Copy/paste the whole block; it pings vehu's broker, saves the captured RPCs, shows
them, then leaves the engine pristine (level 1, log cleared):

```bash
export M_YDB_BIN=~/vista-cloud-dev/m-ydb/dist/m-ydb
ENG="--engine ydb --transport docker --container vehu"

v-rpc debug status $ENG                                   # 1. baseline (level 1, off)
v-rpc debug arm    $ENG                                   # 2. arm capture (level 2)
v-rpc debug ping   --addr 127.0.0.1:9430                  # 3. fire real RPCs at the broker
v-rpc debug capture $ENG --out smoke.ldjson \
      --no-clear --duration 2s --restore-to 1            # 4. save buffered RPCs -> file, restore to 1

# 5. examine the output
cat smoke.ldjson
jq -r '.rpc' smoke.ldjson                                 # just the RPC names
jq -r '"\(.seq)\t\(.rpc)"' smoke.ldjson                   # seq + name, in order
jq -s 'group_by(.rpc)|map({rpc:.[0].rpc,n:length})|sort_by(-.n)' smoke.ldjson  # counts

v-rpc debug clear  $ENG                                   # 6. wipe the buffered log
v-rpc debug status $ENG                                   # 7. confirm: level 1 (off), 0 buffered
```

### B. Real CPRS — capture a live login + chart sweep

CPRS runs in the VM; `v-rpc` runs on the host. (If CPRS can't reach vehu's
loopback broker, front it with a relay — see the `vehu-broker-vbox-relay` note;
CPRS then connects to `10.0.2.2:19431`.)

```bash
export M_YDB_BIN=~/vista-cloud-dev/m-ydb/dist/m-ydb
ENG="--engine ydb --transport docker --container vehu"

# 1. start capturing (arms, clears, streams to file). Leave it running.
v-rpc debug capture $ENG --out cprs-login.ldjson --restore-to 1

#   --- in the VM: launch CPRS, sign in as PROVIDER,VERO (CAS123;CAS123..),
#       open patient TEN,PATIENT, click through the chart tabs ---

# 2. back on the host: Ctrl-C the capture  (restores XWBDEBUG to 1)

# 3. examine
wc -l cprs-login.ldjson
jq -r '"\(.seq)\t\(.rpc)"' cprs-login.ldjson | head -20         # the sign-on sequence
jq -s 'group_by(.rpc)|map({rpc:.[0].rpc,n:length})|sort_by(-.n)|.[:20]' cprs-login.ldjson

# 4. clean up
v-rpc debug clear  $ENG
v-rpc debug status $ENG                                   # level 1 (off), 0 buffered
```

> Capture files (`*.ldjson`) are git-ignored — they're data, not source.

## 7. Comparing against the VSL tap

The whole point of the LDJSON format is that its field names line up with the
s3tap envelope (`rpc`, `ts`, `job`, `seq`), so you can join the two captures
**offline** to validate the VSL tap:

1. Capture the oracle: `v rpc debug capture … --out xwbdebug.ldjson` while you run
   a known workload (e.g. a CPRS login).
2. Capture the same workload through the VSL tap (separate pipeline) → its LDJSON.
3. Compare offline — e.g. load both into DuckDB and diff the set of `rpc` names
   per `job`:

   ```sql
   -- RPCs the broker saw (oracle) but the VSL tap missed
   SELECT rpc FROM 'xwbdebug.ldjson'
   EXCEPT
   SELECT rpc FROM 'vsltap.ldjson';
   ```

The correlation/analysis itself lives in the analysis pipeline, **not** in this
tool — `v rpc debug` only produces the comparable oracle output.

## 8. Limitations

- **Names only.** No RPC results, no request↔response correlation, no payloads
  (those need the VSL hook).
- **Can drop traffic.** `XWBDEBUG` writes to `^XTMP("XWBLOG"_$J)`, one node-tree
  per broker job, and **wipes it at the start of each new connection**. Since
  process IDs recycle, a connection that begins *and* ends entirely between two
  polls can be overwritten before the tool reads it. Use a short `--interval` for
  busy systems; for lossless capture use the VSL tap. (A single, persistent CPRS
  session is one connection, so its full sequence is captured reliably.)
- **Self-purging.** Each log auto-expires ~7 days after its connection (Kernel's
  `^XTMP` cleanup) — fine for live use, not an archive.
- **Second-resolution timestamps** (`$HOROLOG`).

## 9. Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| `NO_DRIVER` error | The `m-<engine>` driver isn't found — build it (`make build` in `m-ydb`/`m-iris`) or set `M_<ENGINE>_BIN`. |
| `ENGINE` error from `status` | Engine down or unreachable over the driver — check the container is up and the transport/container flags are right. |
| Nothing appears in `tail` | No traffic is hitting the broker, or your `--filter` excludes it; try `--all`, and confirm with `status` that the level is ≥ 2. |
| Level didn't restore (still `ON`) | Either a hard-kill (`kill -9`) skipped the restore, or an **overlapping run**: `tail`/`capture` restore to the level they *found* at start, so if one started while another already armed level 2, it leaves it at 2. Fix: `v rpc debug disarm`, or run with `--restore-to 1` to force stock on exit. |
| Buffered lines won't go away | They auto-purge in ~7 days; to wipe now use `v rpc debug clear` (or start a `tail`/`capture` without `--no-clear`). |
| Capture file empty | The workload ran outside the capture window, or every connection was wiped between polls — lower `--interval`, or arm first and capture with `--no-clear`. |
| CPRS can't reach vehu | vehu's broker is published loopback-only; front it with a host relay so the VM can reach it (`vehu-broker-vbox-relay` note). |

## References

- **vehu credentials** — [Docker Hub: `worldvista/vehu`](https://hub.docker.com/r/worldvista/vehu)
  (per-user access/verify table); [WorldVistA/docker-vista README](https://github.com/WorldVistA/docker-vista/blob/master/README.md);
  [Hardhats — VEHU/CPRS Demo access/verify codes](https://groups.google.com/g/Hardhats/c/ABYd5QyXmTk).
  Codes were **confirmed against this engine** read-only (`$$EN^XUSHSH` access-code
  hash → `#200` "A" index).
- **CPRS sign-in** — [Signing In to CPRS (WorldVistA)](https://code.worldvista.org/files/clients/OSEHRA_VistA/CPRS/1_0_31_118/Help/Signing_In_to_CPRS.htm).
- **Patient-data figures** — live read-only probe of vehu via the driver seam:
  orders `^OR(100,"AC",DFN)` (#100), notes `^TIU(8925,"C",DFN)` (#8925), visits
  `^AUPNVSIT("C",DFN)` (#9000010), names `^DPT(DFN,0)` (#2). 2026-06-26.
- **Internal** — `docs/v-rpc-implementation-plan.md` (design + tracker);
  `docs/memory/v-rpc-domain.md` (locked design, follow-ups); the shared
  `cprs-rpc-xwbdebug-host-probe` memory and `cprs-rpc-xwbdebug-smoke-test`
  proposal (XWBDEBUG mechanics on the engine side).
