---
name: v-rpc-doctor-relay
description: v rpc doctor (CPRS↔VistA network healthcheck) + v rpc relay (built-in TCP forwarder replacing socat) — built 2026-06-27, live-verified against vehu; the productized fix for the loopback-broker / VBox reachability nightmare.
metadata:
  type: project
---

**`v rpc doctor` + `v rpc relay` built 2026-06-27** to make the CPRS-in-VBox →
VistA-in-Docker broker connection self-diagnosing instead of tribal knowledge.
Motivated by the recurring `WSAECONNREFUSED / WASConnectByName` failure (every hop
in the chain dies the same opaque way). Supersedes the manual `socat` relay in
[[vehu-broker-vbox-relay]]. Proposal: `docs/proposals/v-rpc-network-doctor.md`.

**Root cause (now machine-readable):** vehu publishes the broker bound to
`127.0.0.1` (`docker inspect` → `HostIp:127.0.0.1`), so a VM can't reach it
directly. `doctor` reads that binding and *explains* it instead of leaving it
mysterious.

**`v rpc doctor`** — walks the chain `docker → broker publish mode → broker
listener → relay`, one structured Check per hop (ok/warn/fail/info) with a
plain-language detail + exact Fix, then derives the **CPRS address** (vehu:
`10.0.2.2:19431`). `--fix` starts the relay if needed+missing and re-checks.
`-o json` emits the full report (the `Report` struct) for scripts/agents. Never
touches vehu/VistA/the VM.

**`v rpc relay`** — dependency-free Go TCP forwarder (`net.Listen` +
bidirectional `io.Copy`), **no socat**. Discovers the backend from `docker
inspect`; default `0.0.0.0:19431 → 127.0.0.1:9430`. `--install` writes a
`systemd --user` unit (`v-rpc-relay.service`, ExecStart = the resolved binary);
`--status`/`--uninstall`. Linux auto-installs; other OSes get printed
instructions.

**Architecture (TDD, leaf-first):** `internal/relay` (forwarder, 95.8% cov) and
`internal/netcheck` (pure ladder over injected `Docker` + `Prober` seams, 86.8%)
are fully unit-tested with **no engine/docker/network**. Real adapters in
`rpccli/netadapters.go`: `dockerInspect` (shells `docker inspect` — NOT `docker
exec`, so the engine-stack guard is satisfied) and `xwbProber` (dials + sends one
no-arg `XWB IM HERE`, reuses `internal/xwbwire` — the same RPC-client wire path as
`ping`, **not** the engine driver seam; rule-3 transport monopoly governs M
*execution*, not a dumb socket probe/forwarder). Verbs are top-level under the
**Connect** group in `Commands`.

**GOTCHAS found + fixed:**
- Docker's "no such object" is **lowercase** in this engine's version — match
  case-insensitively or a missing container reads as "docker broken".
- A live `xwbProber` must treat **0 reply bytes as failure** (docker proxy accepts
  the TCP but the M listener never answered) — else a dead listener reads as OK.
- **Makefile bug fixed same day:** `BIN ?= v-rpc   # comment` baked trailing
  spaces into `BIN`, so `make build/install` produced a binary literally named
  `v-rpc<spaces>`. Comment moved to its own line.

**Live end state (2026-06-27):** binary installed (PATH `v-rpc` is a symlink to the
repo-root build); the hand-made `vehu-broker-relay.service` was retired and replaced
by `v-rpc relay --install` → `v-rpc-relay.service` (enabled, active, linger on).
`v rpc doctor` reports the path green; CPRS connects `s=10.0.2.2 p=19431`.

**Open (owner):** confirm the three proposal questions (waterline reading, home of
the verbs, `--fix` scope). Committed straight to `main` (gate green) per the org
trunk-based protocol.
