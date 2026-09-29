# cmd/orbit

## What it is

The composition root and the binary's entry: the runtime's main with
orbit's tools, the `/earn` and `/projects` commands, the orbit title, and
the subcommands
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
  root, the five orbit tools behind the lazy client seam, the `/earn` and
  `/projects` commands, the swarm adapter, the earn footer snapshot, and the shipped
  theme (the embedded default when the home has no theme.json; a home
  theme.json wins entirely).
- **The lazy seam** (`clientProvider`): the first tool use loads the agent
  config and fails loudly, naming `/earn`, when it is missing; a failed
  load is retried at the next use, so `/earn`'s init can fix it in the
  same session. `Read` loads read mode instead (indexer + RPC, no agent
  key) — the seam the `projects` tool and command use. The operator's
  vault authority key never enters the process.
- **The subcommands**: `init` (hot wallet + config + airdrop), `vault`
  (create/deposit/withdraw/link/unlink/show), `project` (create/list),
  `board` (read/act), `agent` (register/refresh/show/list), `snapshot`,
  `bootstrap` (the unsigned operator handoff), `run-job` (the fire path).
- **The seam closures**: `wire` (the kernel), `swapIn`, `switchModel`,
  `switchRole`, `switchApprove`, `newSession`, `sessionResume`, the
  plugin reload/swap, `statusIn`/`earnRows` (the footer, painted dim),
  and the `command.Env` closures over the root.
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
- **The fire path** (`runJobFire`): the job's identity row -> the hot
  wallet floor check (`checkFireFunded`: under 0.005 SOL it refuses with
  the same line the earn preflight prints) -> the live read snapshot ->
  `brief.Build` at the row's size -> the job prompt refresh -> spawn. The
  fire's sandbox is always on (landlock — the netless profile, and the
  only one that runs unprivileged), the interactive setting never reaches
  it, and the agent id rides the scratch (`writeFireAgentID`) so the
  worker's `Store.OnAct` hook keeps the footer's act snapshot. The
  sandbox is netless by design, so the fire's chain traffic tunnels
  through `chainTunnel` (`<home>/.rig-job-chain.sock`): the worker's
  client dials the socket and the proxy forwards the TLS bytes to the
  host named in the ClientHello. The footer snapshot is best-effort: a
  failed snapshot write never kills the fire. The fire writes it at fire
  start (the read state), after every board act (the worker's `Store.OnAct`
  hook refreshes it with the act's role, verb, and task id), and at fire
  end.
- **The fire's fixed wire**: the jailed worker names itself by the
  scratch home (`isFireJail`: `RIG_HOME` ending in `.rig-job`), resolves
  the orbit home (`os.Setenv("RIG_HOME", ...)`), and pins both the wire
  and the allow-list to `fireToolNames` — `project`, `intel`, `wallet`,
  `board`, `projects`, `read`, `rem`, `bash`, `python`, `todo` — with no scheduler,
  plugin, sessions, or delegate, and no plugin/python wiring at all. The
  operator's interactive allow is untouched: only the fire worker
  resolves to the ten. The key guard (`guard.go`) refuses a tool call that
  names the hot key; landlock falls back to the operator's sandbox with one
  line where the kernel lacks it; the chain tunnel forwards only the
  indexer, RPC and airdrop hosts.
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

- The orbit tools are natives: `orbitToolNames` rides `nativeToolNames`
  (the wire toolset is built from that table) and `appendOrbitTools`
  admits them to an allow-list that names none of them. A tool that is
  registered but not named there is invisible to the model. The fire is
  the exception by design: `fireToolNames` is the fire's whole wire.
- The fire's floor, jail, and tunnel are rig-shaped: the scratch home
  (`<job cwd>/.rig-job`), the netless sandbox, and the socket proxy all
  come from the pinned rig v2.1.1. A rig upgrade that renames the scratch,
  stops pinning `RIG_HOME`, or changes the netless guarantee must be
  checked here first.
- The scheduler's crontab tag is scoped to the rig home (`home`): the
  orbit home rides through `agent.Register`/`Refresh`/`Fire`, the earn
  command's `Home`, and the `agent` and `run-job` paths, and the store's
  migration is `Migration(schedHome, cfgDir, RunnerCommand(self),
  RealCrontab(""))` — the scheduler home for the old pane-scheduler
  stores, the orbit home for the tag, and the runner command so a line is
  claimed by key and command. The menu changed with rig v2: `ls`, `find`,
  `grep`, and `diff` are gone (`read` is the observation path), and
  `web_search`/`web_fetch` are the one `web` tool.
- The fire's sandbox refuses when the box cannot provide it (landlock ABI
  < 4, or a kernel without the netless guarantee): no sandbox, no fire.
