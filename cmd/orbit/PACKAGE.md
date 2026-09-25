# cmd/orbit

## What it is

The composition root and the binary's entry: the runtime's main with
orbit's tools, the `/earn` command, the orbit title, and the subcommands
(init, vault, project, board, agent, snapshot, bootstrap, run-job). Every
dependency is explicit in one place; it is the only package that imports
the whole tree. SPEC_EARN's decision 1 governs the shape: the binary's
default path is the interactive one, and the subcommands stay behind
dispatch.

## What it includes

- **main()**: the runtime path — flag/env/file config, the orbit home,
  the five stores, the python kernel, plugin discovery, the canonical
  middleware chain, the frontend selection (`-tui` auto / one-shot / plain
  CLI) — plus the orbit wiring: the identity and board stores at the home
  root, the four orbit tools behind the lazy client seam, the `/earn`
  command, the swarm adapter, and the earn footer snapshot.
- **The lazy seam** (`clientProvider`): the first tool use loads the agent
  config and fails loudly, naming `/earn`, when it is missing; a failed
  load is retried at the next use, so `/earn`'s init can fix it in the
  same session. The operator's vault authority key never enters the
  process.
- **The subcommands**: `init` (hot wallet + config + airdrop), `vault`
  (create/deposit/withdraw/link/unlink/show), `project` (create/list),
  `board` (read/act), `agent` (register/refresh/show/list), `snapshot`,
  `bootstrap` (the unsigned operator handoff), `run-job` (the fire path).
- **The seam closures**: `wire` (the kernel), `swapIn`, `switchModel`,
  `switchRole`, `switchApprove`, `newSession`, `sessionResume`, the
  plugin reload/swap, `statusIn`/`earnRows` (the footer), and the
  `command.Env` closures over the root.
- **Resolution helpers**: `client.Home` (the orbit home, `RIG_HOME`
  overrides), `resolveModel`, `sessionFor`, `checkOneShot`, `splitCSV`,
  `effectiveNativeNames`, `registeredNativeNames`, `isMutating`.

## How it is consumed

- **The orbit tools are lazily wired**: the TUI must start before orbit is
  set up (that is what `/earn` is for). The worker path still fails closed
  at startup: a fire without a config never spawns.
- **The middleware order** (`canonicalMiddleware`) is the one place the
  order is written down: toolset resolves the live table, approve gates
  (auto unless a frontend can ask), cutoff refuses a truncated call
  before approval spends a prompt, perm denies before approval asks and
  before the retry guard counts a failure, guard bounds retries/rounds/
  results, and paths expands `~` outermost so every tool inherits it.
- **The fire path** (`runJobFire`): the job's identity row -> the live
  read snapshot -> `brief.Build` at the row's size -> the job prompt
  refresh -> spawn. The footer snapshot is best-effort: a failed snapshot
  write never kills the fire.
- **Reads stay keyless**: project list, agent list, vault show, and board
  read load read mode (RPC only, no vault creator, no agent key).
- **Bootstrap** prints the unsigned vault-admin instructions as JSON for
  the operator to sign and send with the vault authority key; the process
  never holds it.

## Gotchas

- The earn footer is a local snapshot (`~/.orbit/status.json`), never the
  chain at status-callback time: `/earn status` and each agent fire write
  it, the status callback only reads the file, so every command stays off
  the network.
- One home: the hot key, the client config, the identity rows, the board
  cache, and the status snapshot all live in the orbit home (`~/.orbit`;
  `RIG_HOME` overrides, `~/.rig` stays rig's — the binary pins a rig
  version, so its stores never share migrations with the standalone rig).
- The operator key is named per call (flag > env) and never stored.
- `-version` reads the runtime's module version from the build info — the
  number is never hardcoded.
- Bootstrap's unsigned account lists mirror the IDL; keep them in sync
  with the client builders (`client/vault.go`).
- `run-job` lands before the session wiring: it is its own lifecycle
  (own identity row, own stores, own record) and must not touch the
  interactive closure order.
- The `-p`/`-resume` conflict refuses loud before any store opens
  (one-shot stays one-shot).
