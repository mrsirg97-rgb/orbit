# orbit: earn

Earn is the one-minute join path as a slash command, and the TUI that
ships it. `cmd/orbit` is rig's main with orbit's tools: the same kernel,
stores, middleware, and frontend selection (auto TUI / piped CLI /
one-shot worker), plus `market`, `intel`, `wallet`, and `board` behind the
natives, the `/earn` command beside `command.All()`, and the orbit title
("powered by rig").

## definition

- **The TUI**: `tui.New(os.Stdin, os.Stdout, theme, WithTitle("orbit",
  orbitRows, "powered by rig"), WithCommands(command.All()+earn, env),
  WithStatus(orbitStatusIn), WithStatusTick(2 * time.Second))`. The
  theme is orbit's shipped one: the embedded default when the home has
  no theme.json (a home theme.json wins entirely). The status function
  adds the earn rows under the footer — painted dim, the same grey as
  the model rows: projects held, open claims, last memo, earnings since
  start. The same rows are what `/earn status` prints. The tick re-reads
  the status function while the input loop is idle (zero is off; a
  fire's write shows up without a command).
- **The hint**: the command's description names the moments — `/earn
  (status, or join as a worker), join [roles], roles [add|remove
  <role>], goal "<paragraph>", status, stop, start` — and the
  unknown-action error lists the same verbs.
- **bare /earn** (the wizard's door): prints status when the home is set
  up (hot key, vault, link, deposit), otherwise runs join with one
  worker.
- **/earn join [roles]**: runs the setup (init when there is no hot key,
  then the vault steps) and registers the roles (worker by default). The
  operator key is named at the call via `--operator-key` /
  `--operator-key-path` / `ORBIT_OPERATOR_KEY(_PATH)`; at the first
  signing step the path is asked once and remembered as
  `ORBIT_OPERATOR_KEY_PATH` in the config — the path, never the key.
  Before any chain spend, one preflight line names what exists and what
  will happen; the preflight also reads the hot wallet balance and, under
  the 0.005 SOL floor, names the pubkey and `send devnet SOL or wait for
  the faucet` — the hot wallet pays every write's rent and fees. A wallet
  under the floor retries the airdrop through init, and the setup refuses
  before any chain spend if the retry leaves it there. The setup is safe
  to rerun: create checks `TorchVaultPDA(creator)` on chain against the
  recorded creator and records the creator without sending when the vault
  exists, link and deposit confirm the signature before returning, and
  deposit is gated on the recorded `ORBIT_VAULT_DEPOSITED` marker or the
  vault record's `total_deposited` (never the running balance). The model
  check runs before any chain spend; a role whose job exists is refreshed,
  otherwise created.
- **/earn roles**: lists the roster. **/earn roles add|remove <role>**:
  registers one role and its job, or removes the role's identity row and
  job.
- **/earn goal "<text>"**: sets or changes the architect's goal,
  registering an architect (and its job) when none exists.
- **/earn status**: prints the footer rows; without a config it prints the
  local snapshot when one exists (else one line: not set up yet), never
  the chain client. The six moments ride `command.Subber`, so the TUI's
  picker lists them. In the footer the labels are ember (the accent), the
  values are text, and the PnL value is success when positive, error when
  negative. **/earn stop**: pauses the
  jobs. **/earn start**: resumes them.
- **Reads stay keyless**: project list, agent list, and vault show load
  read mode (RPC only, no vault creator, no agent key).

## decisions

### 1. cmd/orbit is rig's main, not a one-shot wrapper

The binary's default path is the interactive one: config load (the orbit
home: settings, models, workers, plugins), the five stores, the python
kernel, plugin discovery, the canonical middleware chain, the native tool
table plus the four orbit tools, and the frontend door (`--tui auto`:
TUI when stdout is a terminal, the piped CLI otherwise, the one-shot
worker with `-p`). The subcommands (init, vault, project, board, agent,
snapshot, bootstrap, run-job) stay behind dispatch; the scheduler's fire
path is `orbit run-job <key>`.

### 2. The orbit tools are lazily wired

The TUI must start before orbit is set up (that is what /earn is for), so
the orbit tools take a client provider: the first use loads the agent
config and fails loudly with a named error ("run /earn") when it is
missing. The worker path still fails closed at startup: a fire without a
config never spawns.

### 3. Identity rows are per (wallet, role), roles are local

Schema v5: one row per (wallet, role), id = `@APxxxx-role`; the role owns
cadence, brief size, budget, stall, and timeout defaults; the goal is an
architect-only column carried into the brief's YOU ARE section. No
archetypes, no voice, no stake scale — the memo grammar stays role-free
(SPEC_BOARD: the verb says what happened; the sender is the only identity
the fold trusts).

### 4. The footer rows are a local snapshot, never the chain

`earn.Status` reads the wallet (held projects, earnings since start) and the
board cache (the wallet's open claims, its last memo) — command time
only. `/earn status` and each agent fire write the rows to the local
snapshot (`<orbit home>/status.json`), and the TUI's status callback
reads only that file. Every command recaptures the status in rig, so a
callback that touched the chain would put `/help` on the network; the
snapshot keeps the footer local. The rows are as fresh as the last
`/earn status` or fire — a mid-turn tool effect shows stale until the
next command, and the brief's own numbers (the fire's read) carry the
same staleness.

A fire writes three times, all best-effort: at fire start the rows come
from the fire's read state (`RowsFromBrief`, no extra chain reads), after
every board act the worker re-reads the wallet and the board cache (the
act just landed) and records the act as the snapshot's last fire (role,
verb, task id, time), and at fire end the parent writes the fresh rows
once more. The footer's last-memo row shows the last fire — `last memo:
3m ago · "claim 7" · worker claim #7 · just now`. The snapshot write is
atomic under concurrency: `os.CreateTemp` in the snapshot's directory,
then rename, so two fires writing at once never leave a truncated file.

### 5. The wizard asks with the answers at the call

Rig's command model has no free-text ask seam (the Ask door is y/n), so
the wizard prints its questions and expects the answers on the line:
`/earn architect worker --goal "..."`. A missing role set or an architect
without a goal refuses with the question, naming the exact follow-up.

### 6. The brief's vocabulary is torch's, not Pyre's

PNL for HLTH; back/exit/post/pass for the opcodes (no glyphs);
bonding/ready/migrated (and reclaimed) for the status words;
held/founded/sentiment for MBR/FNR/SENT; "other contributors" for
rival/ally; "brief" for "world block". The compact brief drops the
realized-PnL line and the voice lines, so it shrinks; the goldens pin the
new bytes.

## layout

- `earn/command.go` — the /earn command: register wizard, status, stop,
  start
- `earn/status.go` — the footer rows (held, claims, last memo, PnL), the
  last fire (role, verb, task id, time), and the local snapshot (atomic
  write at `/earn status` and per fire, read at status callback)
- `earn/earn_test.go` — wizard, status, stop against fakes
- `cmd/orbit/main.go` — rig's main with orbit tools, /earn, and the title
- `cmd/orbit/*.go` — the subcommands (init, vault, project, board,
  agent, snapshot, bootstrap, run-job)
- `identity/` — v5: role + goal, per-(wallet, role) rows
- `brief/` — the brief builder (formerly `world/`)
