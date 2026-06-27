# Memory index — v-rpc (per-repo)

Per-repo memory for the `v rpc` domain, committed with the code (org per-repo
memory model). One line per entry; detail in the linked file.

- [v-rpc domain](v-rpc-domain.md) — what `v rpc debug` is, the locked design, the waterline-clean architecture, and the deferred v-cli mount (needs a published+tagged repo first).
- [v rpc doctor + relay](v-rpc-doctor-relay.md) — the CPRS↔VistA network healthcheck (`doctor`) + built-in TCP forwarder (`relay`, replaces socat). Built+live-verified 2026-06-27; root cause = loopback broker publish (machine-readable via `docker inspect`). Productizes [[vehu-broker-vbox-relay]].
