# v-rpc-debug — the `v rpc` domain

VistA RPC developer tools. Today it carries **`v rpc-debug`**, which taps the RPC
Broker's *native* `XWBDEBUG` log over the **m engine driver seam** to view live
RPC traffic in the terminal or save it to a file for **offline comparison against
the Phase-2 VSL tap**.

It is a **debug/validation** tool: validate the VSL tap captures correctly,
troubleshoot capture, and troubleshoot CPRS RPC generation. `XWBDEBUG` is the
zero-install *oracle* — the durable, egress-to-S3 tap is the VSL hook (separate
work). Correlation of this capture with the S3 tap is done **offline and
separately**; this tool only produces comparable output (LDJSON whose fields —
`rpc`, `ts`, `job`, `seq` — align with the s3tap envelope).

## Contents

- [Commands](#commands)
- [Architecture (waterline-clean)](#architecture-waterline-clean)
- [Build](#build)
- [User guide](docs/v-rpc-user-guide.md) · [Implementation plan](docs/v-rpc-implementation-plan.md)

## Commands

**Connect** — get CPRS talking to VistA (the most common stumbling block):

```
v rpc-debug doctor                                           # diagnose the CPRS↔VistA network path; print the fix + CPRS address
v rpc-debug doctor --fix                                     # ...and start the relay if it's needed and missing
v rpc-debug relay  --install                                 # built-in TCP forwarder (no socat); persistent systemd --user service
```

**Capture** — `v rpc-debug`, the XWBDEBUG tap:

```
v rpc-debug status   --engine ydb --container vehu     # current XWBDEBUG level + buffered jobs
v rpc-debug tail     --engine ydb --container vehu     # live viewer in the terminal (Ctrl-C)
v rpc-debug capture  --engine ydb --container vehu --out rpc.ldjson   # save LDJSON for offline analysis
v rpc-debug arm      --engine ydb --container vehu     # turn capture on (XWBDEBUG level 2)
v rpc-debug disarm   --engine ydb --container vehu     # restore (level 1 = stock)
v rpc-debug clear    --engine ydb --container vehu     # wipe the buffered XWBLOG (pristine)
v rpc-debug ping     --addr 127.0.0.1:9430            # fire test RPCs so a tap has traffic
```

The engine flags also read env vars — `export VRPC_ENGINE=ydb VRPC_TRANSPORT=docker
VRPC_CONTAINER=vehu` (e.g. via direnv) and drop them from the command line. See the
[user guide](docs/v-rpc-user-guide.md#2-selecting-the-engine).

Shared flags on `tail`/`capture`: `--all` (every log line, not just `RPC:`),
`--filter TEXT` (RPC-name substring), `--interval` (poll seconds), `--duration`
(bounded run, e.g. `30s`), `--level {2,3}` (3 logs params = **PHI**), `--keep`
(leave armed on exit), `--no-clear` (keep existing buffered log).

Engine selection is **explicit** (`--engine ydb|iris`, required): ydb/vehu for
development (has data), IRIS-VistA for VA validation. The connection
(container/credentials) is read by the driver from its `M_<ENGINE>_*` environment;
`--container` is a convenience that sets `M_<ENGINE>_CONTAINER`.

## Architecture (waterline-clean)

- `internal/xwblog` — pure parse/record/LDJSON/dedup (no engine dependency).
- `internal/capture` — arm/disarm + poll-read + dedup, over a small `Execer`
  interface (fake-tested).
- `internal/relay` — dependency-free TCP forwarder (replaces ad-hoc socat).
- `internal/netcheck` — pure `doctor` check ladder over injected Docker + Prober
  seams (fake-tested; the real adapters — `docker inspect`, the `[XWB]` probe —
  live in `rpccli`).
- `rpccli` — the clikit command surface; adapts `mdriver.Client` to `Execer`. The
  importable package the `v` umbrella mounts as `v rpc`.
- `main.go` — the standalone `v-rpc-debug` binary.

Engine access is **only** through `mdriver.Client` (the m-driver-sdk seam,
waterline rule 3) — never raw `docker exec`. Layer `v`.

## Build & install

```
make check                       # gofmt + lint + race tests + build (pre-commit gate)
make build                       # -> dist/v-rpc-debug
make install BINDIR=~/scripts/bin   # install onto PATH
```

`v-rpc-debug` is one static binary. Put the `m-ydb`/`m-iris` driver in the **same** PATH
directory and `v-rpc-debug` auto-locates it (no `M_<ENGINE>_BIN` needed). Then the only
config is the container — `export VRPC_CONTAINER=vehu` (engine defaults to `ydb`,
transport to `docker`) and run flagless: `v-rpc-debug status`. See the
[user guide §1](docs/v-rpc-user-guide.md#1-setup-one-time).

Caveat (inherent to XWBDEBUG): `^XTMP("XWBLOG"_$J)` is per-handler and wiped at
each connection start, so a connection that begins and ends entirely between two
polls can be missed. Fine for "is traffic flowing" and oracle comparison; the
lossless, durable tap is the VSL hook.

See `docs/v-rpc-user-guide.md` (full usage) and
`docs/v-rpc-implementation-plan.md` (design + tracker).
