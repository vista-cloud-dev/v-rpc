---
title: v-rpc user guide — viewing and saving live RPC traffic with `v rpc debug`
status: draft
version: v0.1.0
created: 2026-06-26
last_modified: 2026-06-26
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
  comparison — see [Comparing against the VSL tap](#comparing-against-the-vsl-tap))
- *Why is this client misbehaving — which RPC fires, in what order?*

> **What it is not.** This is a **debug/validation** tool, not a durable capture
> pipeline. `XWBDEBUG` logs RPC **names** (not results, not request↔response
> correlation), it can drop traffic under load (see [Limitations](#limitations)),
> and it has no S3/egress. The durable, complete, egress-capable tap is the VSL
> broker hook; `v rpc debug` is the zero-install **oracle** you check it against.

---

## 1. Prerequisites

- A reachable engine driven through the **m-driver-sdk seam** — `v rpc` never
  touches the engine any other way. You need the `m-<engine>` driver binary on
  `PATH` or pointed to by `M_<ENGINE>_BIN`:

  ```bash
  export M_YDB_BIN=~/vista-cloud-dev/m-ydb/dist/m-ydb     # for ydb
  # export M_IRIS_BIN=~/vista-cloud-dev/m-iris/dist/m-iris  # for iris
  ```

- The target engine up and healthy (e.g. the `vehu` YottaDB-VistA container).
- The `v-rpc` binary built:

  ```bash
  cd ~/vista-cloud-dev/v-rpc && make build      # -> dist/v-rpc
  ```

Once mounted into the `v` umbrella, every command below is also available as
`v rpc debug …`. Standalone, it is `v-rpc debug …`.

## 2. Selecting the engine

Engine selection is **explicit** — there is no default, on purpose (ydb/vehu has
data for development; IRIS-VistA is the target for VA validation). Every command
takes:

| Flag | Meaning | Typical value |
|---|---|---|
| `--engine` | which engine: `ydb` or `iris` (**required**) | `ydb` |
| `--transport` | driver transport: `local`, `docker`, `remote` | `docker` (default) |
| `--container` | container/instance name; sets `M_<ENGINE>_CONTAINER` | `vehu` |

```bash
v-rpc debug status --engine ydb --transport docker --container vehu
```

(The connection — container, base URL, credentials — is otherwise read by the
driver from its `M_<ENGINE>_*` environment, exactly like `m vista`.)

## 3. The commands

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

### `arm` / `disarm` — explicit control

`tail` and `capture` arm and restore automatically, so you usually don't need
these. Use them when you want `XWBDEBUG` left on across several separate steps
(e.g. arm, run a client by hand, then capture with `--no-clear`).

```bash
v-rpc debug arm    --engine ydb --container vehu          # set level 2 (names)
v-rpc debug disarm --engine ydb --container vehu          # restore to level 1 (stock)
```

## 4. Common flags (`tail` and `capture`)

| Flag | Default | What it does |
|---|---|---|
| `--all` | off | Show every log line (connect/reject/etc.), not just `RPC:` lines |
| `--filter TEXT` | — | Only RPCs whose name contains `TEXT` (case-insensitive) |
| `--interval N` | `1.0` | Poll the log every `N` seconds |
| `--duration D` | `0` | Stop after `D` (e.g. `30s`, `5m`); `0` = run until Ctrl-C |
| `--level {2,3}` | `2` | Debug level to arm — **see the PHI warning below** |
| `--keep` | off | Leave `XWBDEBUG` armed on exit instead of restoring |
| `--no-clear` | off | Don't wipe the existing log on start (capture what's already buffered) |
| `--out PATH` | — | (`capture` only, required) output file; `file://PATH` or `PATH` |
| `--quiet` | off | (`capture` only) don't echo to the terminal |

### Output format

All commands honor the shared `--output` (`-o`) contract: `text` (human, the
default on a TTY), `json` (machine), or `auto`. For `tail`, `-o json` streams the
same LDJSON records `capture` writes — handy for piping into `jq`:

```bash
v-rpc debug tail --engine ydb --container vehu -o json | jq -r .rpc
```

### ⚠️ PHI warning — level 2 vs 3

- **Level 2 (default)** logs RPC **names only** — safe.
- **Level 3** logs the full call string **including RPC parameters**, which on a
  real VistA is **PHI**, written in cleartext to `^XTMP`. Only use `--level 3` on
  a test system with no real patient data, and clear the log afterward.

## 5. Comparing against the VSL tap

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

## 6. Limitations

- **Names only.** No RPC results, no request↔response correlation, no payloads
  (those need the VSL hook).
- **Can drop traffic.** `XWBDEBUG` writes to `^XTMP("XWBLOG"_$J)`, one node-tree
  per broker job, and **wipes it at the start of each new connection**. Since
  process IDs recycle, a connection that begins *and* ends entirely between two
  polls can be overwritten before the tool reads it. Use a short `--interval` for
  busy systems; for lossless capture use the VSL tap.
- **Self-purging.** Each log auto-expires ~7 days after its connection (Kernel's
  `^XTMP` cleanup) — fine for live use, not an archive.
- **Second-resolution timestamps** (`$HOROLOG`).

## 7. Troubleshooting

| Symptom | Likely cause / fix |
|---|---|
| `NO_DRIVER` error | The `m-<engine>` driver isn't found — build it (`make build` in `m-ydb`/`m-iris`) or set `M_<ENGINE>_BIN`. |
| `ENGINE` error from `status` | Engine down or unreachable over the driver — check the container is up and the transport/container flags are right. |
| Nothing appears in `tail` | No traffic is hitting the broker, or your `--filter` excludes it; try `--all`, and confirm with `status` that the level is ≥ 2. |
| Level didn't restore | The process was hard-killed (`kill -9`) before its restore ran. Re-run `v rpc debug disarm` to set it back to 1, then `status` to confirm. |
| Capture file empty | The workload ran outside the capture window, or every connection was wiped between polls — lower `--interval`, or arm first and capture with `--no-clear`. |

## See also

- `docs/v-rpc-implementation-plan.md` — design + increment tracker.
- `docs/memory/v-rpc-domain.md` — the locked design and owner follow-ups.
- The XWBDEBUG mechanics on the engine side: the shared
  `cprs-rpc-xwbdebug-host-probe` memory and the
  `cprs-rpc-xwbdebug-smoke-test` proposal in the `docs` repo.
