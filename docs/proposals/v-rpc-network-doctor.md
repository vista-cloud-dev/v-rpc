---
title: v-rpc-debug network doctor + built-in relay — make CPRS↔VistA "just work"
status: accepted
version: v0.1.0
created: 2026-06-27
last_modified: 2026-06-27
doc_type: [PROPOSAL]
layer: v
---

# `v rpc-debug doctor` + `v rpc-debug relay` — a transparent, portable fix for the CPRS↔VistA network path

> **Status: IMPLEMENTED 2026-06-27** (owner-authorized build). `v rpc-debug doctor` +
> `v rpc-debug relay` are live and verified against vehu end-to-end. See the increment
> tracker (`docs/v-rpc-implementation-plan.md`) and
> `docs/memory/v-rpc-doctor-relay.md`. The three **Open questions** below remain
> for owner confirmation but did not block the build.

## The problem

Getting CPRS (in a VirtualBox Windows VM) to reach the VistA RPC broker (in a
Docker container) is a multi-hop chain across **three isolation boundaries**, and
**every hop fails with the same opaque error at the CPRS end** —
`WSAECONNREFUSED / WASConnectByName`. There is no single place to see the chain,
and the knowledge needed to fix it (port numbers, the loopback-publish gotcha, the
relay, the `10.0.2.2` NAT alias) is tribal — undiscoverable without an expert (or
an AI) walking it by hand. That is the actual defect: **the failure is invisible
and the remedy is undocumented in the tool.**

### The chain (and where it silently breaks)

```
CPRS (Win VM) ──tcp──► 10.0.2.2:RELAY ──► host:RELAY (socat/relay)
                                              └─► 127.0.0.1:9430 ──► [docker] vehu broker ──► XWB listener (M)
```

| # | Hop | How it breaks | Today's symptom |
|---|---|---|---|
| 1 | vehu container | not running | WSAECONNREFUSED |
| 2 | broker **publish mode** | bound `127.0.0.1` only → VM can never reach it | WSAECONNREFUSED (root cause) |
| 3 | XWB listener (M) | broker job not accepting inside VistA | connect then RST / hang |
| 4 | host relay | not running (today's case) | WSAECONNREFUSED |
| 5 | relay → broker | relay points at wrong backend port | connect then drop |
| 6 | guest route / CPRS config | wrong host/port, or VBox not in NAT mode | WSAECONNREFUSED / timeout |

The **root cause** is hop 2: vehu publishes the broker as `127.0.0.1:9430->9430`
(confirmed via `docker inspect` `HostIp=127.0.0.1`). A host that binds a port to
loopback is unreachable from a VM, so a relay on `0.0.0.0` is required — *unless*
the container is republished on `0.0.0.0`. Crucially, **this is machine-readable**:
the publish mode is right there in `docker inspect`, so a tool can detect it and
explain it.

## Recommendation

Add two verbs to the `v rpc` domain. They reuse what v-rpc-debug already has — the
**driver seam** (engine-side checks, as `status`/`tail` do) and the **`xwbwire`
[XWB] client** (socket-side checks, as `ping` does) — so they are a natural
extension, not a new subsystem.

### 1. `v rpc-debug relay` — a built-in, dependency-free TCP forwarder

Replace ad-hoc `socat` with ~30 lines of Go (`net.Listen` + bidirectional
`io.Copy`, one goroutine per connection). Why built-in:

- **No external dependency.** socat isn't on every machine; Go stdlib is in the
  binary. The next user needs nothing but `v-rpc-debug`.
- **Portable.** The next user may be on macOS or WSL; one Go forwarder covers all.
- **Discoverable backend.** Default `--to` is *read from the live docker publish
  binding*, not hardcoded — so it adapts to whatever container/port the user has.

```
v rpc-debug relay                       # listen 0.0.0.0:19431 -> discovered broker (127.0.0.1:9430)
v rpc-debug relay --listen 0.0.0.0:19431 --to 127.0.0.1:9430
v rpc-debug relay --install             # generate a user systemd unit (Linux) / print launchd plist (macOS)
v rpc-debug relay --status / --uninstall
```

`--install` **generates** the persistence artifact (today's hand-made
`vehu-broker-relay.service`) transparently and reversibly — it is not a hidden
side effect, and it is host plumbing, **not** an M-engine installer (the
"never a bespoke installer" directive governs KIDS/engine installs, not this).

### 2. `v rpc-debug doctor` — the end-to-end network healthcheck (headline)

One command that walks the chain **engine → host → relay → guest** and prints, per
hop: what it checked, the result (✓/✗), and the **exact remediation**. Each check
is independently meaningful, so a red line tells you precisely which boundary
broke.

```
$ v rpc-debug doctor --engine ydb --container vehu
✓ docker          vehu is running (image worldvista/vehu)
✓ broker publish  9430 published — but bound 127.0.0.1 (loopback only) → a relay is required for a VM
✓ broker listener XWB handshake on 127.0.0.1:9430 replied 52 bytes (listener live)
✗ relay           nothing listening on 0.0.0.0:19431
                  → run `v rpc-debug relay` (or `v rpc-debug relay --install` for always-on)
…
CPRS should connect to:  s=10.0.2.2  p=19431     (VirtualBox NAT)
```

Check ladder:

1. **docker / engine** — container running? (`docker inspect`) → fix: start it.
2. **broker publish mode** — read the binding `HostIp`. `0.0.0.0` ⇒ VM-reachable
   directly, *no relay needed*; `127.0.0.1` ⇒ loopback-only, relay required. **This
   is the check that explains the whole failure.**
3. **broker listener live** — TCP-connect to the published port + a minimal [XWB]
   `XWB IM HERE` handshake (reuse `xwbwire`/`ping`); a reply proves the M listener
   is actually accepting, not just the docker proxy. → fix: listener down in VistA.
4. **relay present** — if loopback-only, is a non-loopback listener up on the relay
   port? → fix: `v rpc-debug relay` / `--install`.
5. **relay forwards** — run the same handshake *through* the relay → proves the full
   host path end-to-end.
6. **guest target** — print the exact CPRS string for the detected/declared VBox
   mode (NAT ⇒ `10.0.2.2:RELAY`), with the mode caveat. (Doctor can't run inside
   the guest; it states the expectation.)
7. **firewall** (best-effort, advisory) — ufw/iptables block on the relay port?

`v rpc-debug doctor --fix` performs only the **safe, reversible** auto-fixes (start the
relay) and prints the CPRS connection string + the VirtualBox shortcut. It **never
touches vehu, VistA config, or the guest** — diagnosis and host plumbing only.

### 3. Discover, don't hardcode

The magic numbers become discovered or documented-and-overridable:

| Value | Source |
|---|---|
| broker host:port | **discovered** from `docker inspect` publish binding (adapts per user) |
| relay addr | default `0.0.0.0:19431`, `--listen` / `VRPC_RELAY_ADDR` |
| guest host | VBox-NAT convention `10.0.2.2`, stated + overridable for bridged/host-only |
| container | existing `VRPC_CONTAINER` |

Net effect: the next user runs **`v rpc-debug doctor`**, is *told* their topology and the
one thing to fix, and runs **`v rpc-debug relay`** (or `doctor --fix`). No tribal
knowledge, no AI required.

## Why this shape

- **Transparent** — every check prints its reasoning and remediation; the relay is
  visible plumbing; `--install` generates an inspectable unit file.
- **Portable** — no socat/external deps; relay is Go stdlib; persistence generated
  per-OS; ports discovered, not assumed. Works for a fresh machine.
- **Reuses the domain** — driver-seam checks + `xwbwire` client already exist in
  v-rpc-debug; doctor/relay sit naturally beside `ping`.
- **Educational** — doctor names the *root cause* (loopback publish) and offers
  both the principled fix (republish `0.0.0.0`, no relay) and the non-invasive one
  (relay), so the user understands the topology.

## Open questions (for the owner)

1. **Waterline / rule 3.** v-rpc-debug already reaches the broker socket directly via
   `ping` (`xwbwire`), *not* through the driver seam — the transport monopoly
   governs **M-engine execution**, not being a dumb RPC client / byte-forwarder.
   `relay`/`doctor` extend that already-accepted pattern; confirm that reading is
   fine. (See [[engine-access-through-driver-stack]] / the m/v waterline ADR.)
2. **Home for the tools.** `relay`/`doctor` are host-network tools wearing the
   `v rpc` hat because they're specific to the VistA broker path. Confirm they
   belong in v-rpc-debug vs a generic host tool.
3. **Scope of `--fix`.** Confirm auto-start-relay is the only auto-fix; everything
   else stays advisory (never re-run the vehu container automatically).

## Plan (if accepted — TDD, leaf-first)

- **I1 `internal/relay`** (pure-ish, TDD): bidirectional copy forwarder over a
  `net.Listener`; table tests with in-memory pipes / `net.Pipe`.
- **I2 `internal/netcheck`** (TDD, fake docker-inspect + fake dialer): the check
  ladder as pure functions returning structured results (so `doctor` renders, and
  `-o json` emits them for scripts/agents).
- **I3 `v rpc-debug relay`** — wire I1 + publish-binding discovery; `--install` unit
  generator (user systemd today; launchd plist text for macOS).
- **I4 `v rpc-debug doctor`** — wire I2; human ladder + `-o json`; `--fix` calls relay.
- **I5** — fold into the user guide (replace the hand-run socat steps in §6B/§9
  with `v rpc-debug doctor`/`relay`); update `vehu-broker-vbox-relay` memory.

## References

- `vehu-broker-vbox-relay` memory (the manual socat relay this replaces);
  `cprs-rpc-xwbdebug-host-probe` (broker-side mechanics).
- Confirmed live: vehu publishes `9430` bound `127.0.0.1` (`docker inspect` →
  `HostIp:127.0.0.1`), raw `docker run`, image `worldvista/vehu`. 2026-06-27.
- Existing reuse points: `internal/xwbwire` ([XWB] client), `rpccli/ping.go`
  (socket dial pattern), `mdriver.Client` (engine seam).
