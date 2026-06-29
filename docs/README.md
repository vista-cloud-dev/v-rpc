# v-rpc-debug docs

The `v rpc` domain — a debug/validation tool that taps the RPC Broker's native
`XWBDEBUG` log over the m-driver-sdk seam (`v rpc-debug`), plus the CPRS↔VistA
network helpers (`v rpc-debug doctor` / `v rpc-debug relay`).

## Key docs

- [v-rpc-debug user guide](v-rpc-user-guide.md) — how to view and save live RPC traffic
  with `v rpc-debug`, and connect CPRS to vehu (`doctor`/`relay`).
- [v-rpc-debug implementation plan](v-rpc-implementation-plan.md) — design + the live
  increment tracker (Tier-D).

## Folders

- `proposals/` — design proposals for this domain ([network doctor + relay](proposals/v-rpc-network-doctor.md),
  [durable S3 tap (moved to the central docs repo)](proposals/v-rpc-tap-durable-s3.md)).
- `memory/` — per-repo auto-memory (durable lessons; see [MEMORY.md](memory/MEMORY.md)).
