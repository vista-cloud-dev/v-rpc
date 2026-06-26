# v-rpc — the `v rpc` domain

VistA RPC developer tools. Today it carries **`v rpc debug`**, which taps the RPC
Broker's *native* `XWBDEBUG` log over the **m engine driver seam** to view live
RPC traffic in the terminal or save it to a file for **offline comparison against
the Phase-2 VSL tap**.

It is a **debug/validation** tool: validate the VSL tap captures correctly,
troubleshoot capture, and troubleshoot CPRS RPC generation. `XWBDEBUG` is the
zero-install *oracle* — the durable, egress-to-S3 tap is the VSL hook (separate
work). Correlation of this capture with the S3 tap is done **offline and
separately**; this tool only produces comparable output (LDJSON whose fields —
`rpc`, `ts`, `job`, `seq` — align with the s3tap envelope).

## Commands

```
v rpc debug status   --engine ydb --container vehu     # current XWBDEBUG level + buffered jobs
v rpc debug tail     --engine ydb --container vehu     # live viewer in the terminal (Ctrl-C)
v rpc debug capture  --engine ydb --container vehu --out rpc.ldjson   # save LDJSON for offline analysis
v rpc debug arm      --engine ydb --container vehu     # turn capture on (XWBDEBUG level 2)
v rpc debug disarm   --engine ydb --container vehu     # restore (level 1 = stock)
```

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
- `rpccli` — the clikit command surface; adapts `mdriver.Client` to `Execer`. The
  importable package the `v` umbrella mounts as `v rpc`.
- `main.go` — the standalone `v-rpc` binary.

Engine access is **only** through `mdriver.Client` (the m-driver-sdk seam,
waterline rule 3) — never raw `docker exec`. Layer `v`.

## Build

```
make check     # gofmt + lint + race tests + build (the pre-commit gate)
make build     # -> dist/v-rpc
```

Caveat (inherent to XWBDEBUG): `^XTMP("XWBLOG"_$J)` is per-handler and wiped at
each connection start, so a connection that begins and ends entirely between two
polls can be missed. Fine for "is traffic flowing" and oracle comparison; the
lossless, durable tap is the VSL hook.

See `docs/v-rpc-user-guide.md` (full usage) and
`docs/v-rpc-implementation-plan.md` (design + tracker).
