# Changelog
## [0.6.6] — the session line and the idempotent todo

orbit rides rig 2.1.7, one store change that lands and one CLI change
that does not. The landing one: `complete` on a task already done and
`start` on a task already in progress by the same session (or unowned)
are no-ops that look like success — they answer with the echo a fresh
call would give, the row (`[x]`/`[~]`) and the queue summary, and write
no event and run no compaction. The model had read the old refusal
(`'tN' is done; read-only`) as "I did something wrong" and spent a
thought on it, though there was nothing to fix. The refusals that are
real stay: a foreign session's start of an owned task still refuses
naming the claimer, and done stays read-only for every other verb.
`todo` rides the native table and the fire's fixed wire keeps it, so
both doors get the no-op. The other half — outside a repo the session
line now adds "name project on todo and rem calls, as a path to the
repo the work is in" — lives in rig's own session section, and orbit
composes its own, so it does not land here. No behavior changed
elsewhere: the suite is the gate at every commit.

## [0.6.5] — the link fill and the API reply

orbit rides rig 2.1.6, two tool fixes that both land on the native
table. The `todo` tool treated any string in `requires` or `blocks` as
a link target, so a create carrying `requires: ""` or `blocks: ""` — a
model filling every field — refused with `'' not found`, and the model
then believed a link needed a second call. An empty string is the same
as omitting the field now: no link, no refusal, the schema's link
descriptions end with "omit when none", and the todo sentence names the
one-create sibling rule. `web fetch` returned a JSON body as raw text
up to the cap, so a 77 KB API reply was unreadable and the model fell
back to curl and jq. A JSON response now comes back as one shape line
first — the top-level type, its keys, each array's length and its first
element's shape — then the compacted JSON, both under the same cap and
[TRUNCATED] marker. Both tools are on orbit's wire: `todo` and `web`
ride the native table for every session, and a fire's fixed toolset
keeps `todo`, so the link fix reaches a fire too. No behavior changed
elsewhere: the suite is the gate at every commit.

## [0.6.4] — the approve is an update

orbit rides rig 2.1.5, three dashboard patches and one operator-door
change. The dashboard got the phone: the token cookie lives 90 days and
the web manifest is served per request with the token in its start_url
(2.1.2), the composer's status row and the sessions list are
phone-shaped (2.1.2), and the top safe-area inset moved from the sidebar
to the main column (2.1.3), then into the rules that apply, with a test
that refuses a version that drops it (2.1.5). Orbit does not serve the
dashboard, so none of that touches it. The one door that does:
`/plugins approve` of a name already installed is an update now — one
atomic rename over the installed file, no refusal, no replace flag, and
the reply names the replacement. The agent-facing `plugin` and `plugins`
tools are untouched, and the fire's fixed wire is untouched. No behavior
changed on the runtime path: the suite is the gate at every commit.

## [0.6.3] — a second pass

orbit rides rig 2.1.1, a patch with no code path change. Every tool
description gets a second pass in the 2.1.0 voice: an opening sentence
that says what the tool does, Guidelines that say when and how, a Reply
that names only what a good call returns, with the refusals moved up to
where the model decides. plugin and plugins get their first rewrite, and
the pinned wire and view hashes and the recorded goldens move with the
words.

## [0.6.2] — plain words

orbit rides rig 2.1.0, which rewrote every tool description and the
system prompt in plain sentences, one tool per commit: the house shape
stays (what the tool is, a Guidelines sentence, a Reply sentence), the
restatement and the internal voice are gone. No behavior changed — the
suite is the gate at every commit — and rig's menu test reads the real
menu now, since its recorded wire was stale since 2.0.0. The fire's
fixed wire is untouched.

## [0.6.1] — the comments are gone

orbit rides rig 2.0.2, a patch with no behavior change. Every `//` line
is out of rig's Go — implementation and tests alike, the only exceptions
the generated projections and the metadata packages — and each load-bearing
rationale now lives in its package's PACKAGE.md. The pass also split the
monoliths into one-responsibility files: the composition root, store/todo,
frontend/tui's shell, store/scheduler, store/rem, frontend/web, tool/python,
tool/file, tool/todo, provider/openai, swarm, config, tool/web,
frontend/cli, and tool/rem. Nothing moved on the wire: the suite is the
gate at every commit, and the fire's fixed wire is untouched.

## [0.6.0] — the runtime's new menu

orbit rides rig 2.0.1 (the module moved to `github.com/mrsirg97-rgb/rig/v2`),
and the menu the agent sees changed, which is the minor. `ls`/`find`/`grep`
left the runtime with the `fs` tool package: `read` is the observation
path, and the `diff` tool is gone too — the diff rides `read diff:true`
and `edit`'s drift reply. `web_search` and `web_fetch` folded into one
`web` tool (search or fetch in one call). The scheduler tool and the
`/scheduler` command pick up rig's `repair [id]`: one id repairs that
job, no id repairs every drifting one.

The scheduler's crontab lines are tagged by the rig home they are scoped
to (`rig-scheduler:<hash>:<key>`), so the orbit home rides as `home`
through `agent.Register`/`Refresh`/`Fire`, a `Home` field on the earn
command, and the `agent` and `run-job` paths. The scheduler store's
migration takes the scheduler home, the orbit home, the runner command,
and the crontab, and folds the old pane-scheduler stores by key and
runner command. The fire's fixed wire is unchanged: it never had web.

## [0.5.1] — one world, one vocabulary

A pass over every word the agent can see, so nothing in its environment
contradicts anything else. The project id is PID (the last 8 chars of
the mint), never FID; every tool takes it as `project`, never `mint`.
The `market` tool is now `project`: one project, read plus the six acts,
beside `projects` for discovery; the fire wire and the allow list name
it. `intel` keeps its name and is described as the gossip. The agent is
told "turn", never "fire" (the scheduler's word). The PROJECTS table
drops FOUNDED (always F) and COMMIT (a flag the COMMITMENTS line already
carries); the LEGEND defines `"*"` as the memo beside `$` as the PID.

The `projects` tool and command speak the state words (private, funded,
public) in the list, the show, and the filter, print BACKING rather than
treasury, and read the state from one source (`brief.StateWord`). The
wallet tool's actions are earnings, commitments, standing, reputation.
The board's refusals say "the board refuses" and "wrong state", not
"fold" and "foreign"; a memo the cache has not caught up to replies
"pending: the board has not seen it yet"; the indexer and the RPC are
never named to the agent. The footer says earnings since start. The
gossip lexicon learns the acts (invest, contract, work bullish; release,
short, washed bearish). The role names drop the torch prefix and their
bios describe the acts: the architect signs with accept, the worker
contracts or works and releases at a surplus, the reviewer rejects with a
reason and shorts what it can disprove, and only the funder's accept
counts. The site's agent door and sample block say the same.

## [0.5.0] — work is a position

SPEC_WORK lands: two states, six acts, and a claim that is capital. A
project is private until it is funded and public after (torch's
bonding/ready/migrated/reclaimed read as private/funded/public/closed);
the board's work verbs (task, claim, complete, accept, reject, release)
fold only on a public project, while goal, brief, note and post stand in
every state. A claim is honoured only when its transaction carries
capital: a buy above the memo stake is a contract, an
`open_long_via_vault` is work; a claim with only the memo stake is
foreign. The grammar grows by one verb, `release <id>`, which frees a
held task while the same transaction closes the position or sells the
holding. A reject that rides `open_short_via_vault` is recorded with its
collateral beside the verdict. A work claim has no lease: when the
indexer reports its long ended (closed or liquidated), the fold releases
the task; the lease stays for contract claims.

The board store carries the ledger beside the log: the project's torch
status, the carrier of every claim and reject row (read once from the
transaction through the RPC seam), and the indexer's ended long
positions, in hand tables under `extra.sql` (schema v5). The acts are
`Contract`, `Work`, `Release` (a partial close under 10000 bps keeps the
task), `ShortReject`, and `Held`; every act goes through the one path
(sync, refuse what the fold would refuse with the candidate's carrier,
write, confirm, await), and the state gate is refused by name before any
spend. The render names the backing: `contract 0.0050 SOL`, `work #2`,
`short 0.0200 SOL` beside a reject.

The client builds the four position instructions from the IDL's account
lists and flags (`BuildFromIDL`) over the new PDAs (position, user risk,
long SOL vault, short vault), writes them through the vault
(`WritePosition`, refused before migration), picks the next free position
index by probing the PDAs, reads what a transaction carried (`Carrier`),
and reads the ended longs (`LongEnds`). `GetTransaction` now resolves
each instruction's account keys.

The tools speak the acts: `market` reads rate, size, state, backing,
lends, holdings and gossip and acts with invest, contract, work,
release, short, post; `board` keeps task, brief, note, complete, accept,
reject and points claim at the market tool; `wallet` adds the reputation
ledger (invested, released at a surplus, shorts vindicated, accepts
received, washed out). `orbit board contract <id> <sol>` and `orbit
board release <id>` join the subcommand. The discovery count folds an
uncached window against the project's status and the claims' carriers.

The brief is the gig economy: LEGEND carries the six acts and maps
orbit's states to torch's names; EARNINGS, GOSSIP, RATE, SIZE, BACKING,
COMMIT, LENDS replace PNL, SENTIMENT, PRICE, MCAP, TREASURY, LOAN; a
public project under the lending unlock reads "public, not lending yet";
risk is said plainly (borrowed budget with a clock on it, washed out
costs the stake). The compact ACTIONS block points at the LEGEND instead
of repeating it, so the compact brief stays in its token band.

## [0.4.3] — the hot wallet floor and the fire's fixed wire

Every write's rent and fees are paid by the hot wallet, so funding is now
a precondition. The earn preflight reads the hot wallet balance and, under
0.005 SOL, prints the pubkey and `send devnet SOL or wait for the faucet`;
a fire whose hot wallet is under the floor refuses before spawning with
the same line. `/earn` retries the airdrop through init when the wallet is
under the floor, and refuses before any chain spend if the retry leaves it
there.

Fires get a fixed wire: `market`, `intel`, `wallet`, `board`, `projects`,
`read`, `rem`, `bash`, `python`, `todo`; never scheduler, plugin,
sessions, or delegate. The fire worker names itself by the jail's scratch
home (`RIG_HOME` ending in `.rig-job`), resolves the orbit home, and pins
both the wire and the allow-list to the ten; the operator's interactive
allow is untouched. The fire's sandbox is always on (landlock — the
netless profile, and the only one that runs unprivileged) regardless of
the interactive setting, and the fire carries its agent id through the
scratch so the footer's act snapshot keeps working. Because the sandbox
is netless, the fire's chain traffic tunnels through a unix socket proxy
in the orbit home (`chainTunnel`), routed by the TLS server name — the
worker's client dials the socket and the proxy forwards to the named
host.

## [0.4.2] — the orbit tools reach the model

The wire toolset is built from the native name table, and orbit's copy of
that table was the runtime's list verbatim: `market`, `intel`, `wallet`,
`board` and `projects` were registered in the tool map but never named,
so no session — the TUI, the piped CLI, the one-shot worker every fire
spawns — ever sent them to the model, and the brief described tools the
agent did not have. The five now ride `nativeToolNames`; the reads
(`intel`, `wallet`, `projects`) run in the concurrent batch, the acts stay
serial; and the allow-list default admits them (`appendOrbitTools`: an
allow that names none of them gains all five, one that names any is the
operator's and kept). The brief's Tools line names `projects`.

## [0.4.1] — rig v1.6.0

Orbit now builds against rig v1.6.0. The session listing seeks:
`state.ListSessions` aggregated per session through correlated
subqueries over tables with no seek path, and computed them for every
session before the sort and the limit — 10.5s on a 170-session,
24k-message store, the same with a limit of five. Orbit paid it twice:
the claim reap lists sessions on every start, and the session list
reads the workspace's rows once per store.

The state store gains `metadata.ExtraStatements()` beside its generated
DDL, the todo store's pattern: indexes on `messages(session_id, role,
seq)` and `faults(session_id)`, IF NOT EXISTS, so an existing store
gains them on its next open with no schema bump. The listing selects
the n newest sessions first and aggregates those n only, and the
workspace list reads `state.Cwds` (distinct cwd per store) instead of
the full listing. Measured on a copy of the same store: 13ms with the
indexes, 3ms with the limit-first shape.

## [0.4.0] — project discovery

A worker now picks a project before it claims. The `projects` tool and
the `/projects` command list what the indexer knows with the goal, the
status, the treasury float, and the open task count — filtered by status
(bonding|ready|migrated) or to goals only, sorted by treasury descending,
the economic signal. `/projects show <mint|fid>` names the goal, the
treasury, the board summary (n/m done, open claims), and the last three
memos.

Both read the path the project list and the board read already use: the
indexer's markets and message log, the board's pure fold, the RPC
treasury float. `project.List` now folds the open-task count from the
same message read it uses for the goal, and `project.Show` is the one
project's discovery read. No chain writes, and no key — the tool and the
command load read mode (indexer + RPC, no vault creator, no agent key).

The count is cache-first, not window-only: a project the board cache has
synced is read from the walked log (goal, n/m done, open tasks, open
claims — `board.Store.Summary`), an uncached one falls back to the
newest-50 window, and the fallback is bounded at 10 window fetches per
list, given to the highest-treasury uncached projects. The list stays
one markets call plus a bounded handful of message calls as projects
grow, a busy project's count is the walked log where the cache exists,
and a project beyond the budget shows unknown (`-`) rather than a window
it never read. `show` reads the cache summary the same way when the
project has been synced.

`/projects` implements the runtime's `command.Subber` with the two verbs
(list, show), and the list prints as a table, one row per project.

Tests: `project.List` folds the open-task count from the fake indexer's
markets and memos; the tool lists with the count folded and `show` names
the goal, the board summary, and the last three memos; the command
renders one row per project; a project the board cache has synced counts
from the walked log where the newest-50 window is all noise, makes no
indexer message call, and the fallback fan-out is bounded at
`FallbackBudget` with the unknown marker beyond it.

## [0.3.3] — rig v1.5.9

Orbit now builds against rig v1.5.9. The `live` repaint no longer
repeats rows after a phone keyboard shrink and grow: a pane that shrank
under a tall live region capped its cursor-up at the viewport, and when
the pane grew again the next repaint aimed only as far as the trimmed
region it had painted while short — the picker and the status rows stood
twice. `live` now marks a capped aim and resets the viewport on the next
repaint.

## [0.3.2] — the earn picker, and status without a config

`/earn` implements the runtime's `command.Subber`: the TUI offers its six
moments (join, roles, goal, status, stop, start) in the same popup the
plugins and scheduler commands get, each with a one-line description.

The footer band paints labels in the ember slot (orbit's accent) and
values in the text slot, and the PnL value carries its sign as colour: success when positive,
error when negative.

`/earn status` no longer needs the chain client to say something: on a
home without a config it prints the local snapshot when one exists, else
one line saying the home is not set up yet. A client error carries the
`earn:` prefix once (the provider's "no orbit config (run /earn)" was
wrapped a second time).

## [0.3.1] — the board sync window becomes a walk

The board sync was one window of the newest 100 messages, so a fresh
cache never reached the log's goal memo and a project with more than the
window was never fully folded. Sync now walks: the indexer pages
newest-first with `before=<oldest created_at seen + 1s>` (the boundary
second re-fetched, signatures dedupe), the RPC scan pages with the
signature cursor, and the walk stops when a scanned page holds a
signature already cached or comes back short. The scan's short-page stop
and cursor are the page facts — signatures scanned (memo or not) and the
oldest scanned signature — not the memo rows, so a full page of trades
with no memo is not genesis and the walk continues past it. A fresh
cache walks to genesis (the first goal memo lands), a warm cache reads
one page, and the walk runs per recorded source — indexer with the
timestamp cursor, scan with the signature cursor.

The walk is bounded at 50 pages: past it the cache is marked incomplete
(`project_incomplete`, schema v4) and the board render says so — the
goal is the first goal memo in the walked log. The RPC seam gains the
cursor (`GetSignaturesForAddress(..., before)`) and
`TorchClient.MessagesPage`/`ScanMessages` carry it with the page facts.

Tests: a 250-memo fake log syncs fully from empty in 3 pages; the
same-second boundary loses nothing; a warm cache makes one request; the
bound marks the cache incomplete and the render says so; the goal is the
first goal memo in the walked log; the scan walk pages by signature; a
fake curve of 300 signatures with memos only at positions 5, 150, and
290 walks to genesis from empty and folds all three, and a warm cache
stops after one page.

## [0.3.0] — the earn hints, the tool descriptions, and the shipped theme

The `/earn` command's description and its unknown-action error name the
verbs (`/earn (status, or join as a worker), join [roles], roles
[add|remove <role>], goal "<paragraph>", status, stop, start`), and
"register first" reads "join first" everywhere.

The tool descriptions match the code: a board task needs no id (minted
after the sync), an act the fold would refuse is refused before
spending, only the funder's accept counts, and the reply is the
signature, the memo, and the board, or `pending: not yet indexed`, or
the assigned-id line when the fold renumbers; market is one action per
fire.

The footer paints every earn row with the theme's dim slot — the same
grey as the model rows — and orbit ships its theme: when the home has no
theme.json, the TUI resolves from the embedded default (ember #6b7fa3),
and a home theme.json still wins entirely.

Tests: the description and the unknown-action error name the verbs; the
board tool's description contains "minted" and "funder"; earn rows come
back painted; no theme.json -> ember is #6b7fa3; a home theme.json with
its own ember overrides it.

## [0.2.4] — the indexer read falls back to the RPC scan, sticky per project

The board's message read tries the indexer first and falls back to the
RPC scan when the indexer is unreachable (connect error, 5xx, timeout);
a 4xx never falls back — the indexer answered, and its answer is
authoritative. The fallback is per project and sticky: `project_sources`
records which source numbers a project's messages, and a mint's source
never changes while its rows exist. The fallback applies only to a mint
with no recorded source; an outage on a recorded-indexer mint inserts
nothing — the board read serves the cache with `indexer unreachable:
board may be stale` and acts are refused — and a mint first synced by
scan stays on scan until the cache is rebuilt. Each fallback prints one
line naming the switch.

- **The client classifies the fallback condition** — `IndexerUnreachable`
  (connect/DNS errors, timeouts, 5xx), with a typed `HTTPStatusError`
  carrying the indexer's non-200s; `TorchClient.Messages(mint, limit,
  source)` reads a project's messages from the named source.
- **The board records the source** — `project_sources` (one row per
  project, schema v3); the record decides every sync, so an indexer
  recovery never re-sources an already scanned mint.
- **A source change by config** — unsetting `ORBIT_INDEXER` on a
  recorded-indexer mint wipes the mint's messages, tasks, and notes in
  the same transaction and re-syncs under the scan, so the cache never
  holds rows under two numberings at once.
- **`ORBIT_INDEXER` unset = scan only** and a user-set `ORBIT_RPC` is
  verbatim; the README gains a section on running your own torch indexer
  and pointing orbit at it.

Tests: indexer down → the scan serves the read and the source is
recorded; a later indexer recovery does not re-source an already scanned
mint; a 404 does not fall back; an outage on a warm indexer mint inserts
nothing and refuses an act; unsetting the indexer wipes and rewalks;
the mixed-source case is impossible by construction.

## [0.2.3] — the footer goes live

The TUI re-reads the footer snapshot on a 2s tick while idle, and a fire
keeps it fresh end to end:

- **The TUI sets `tui.WithStatusTick(2s)`** — the status callback
  re-reads the local snapshot every 2s while the input loop is idle, so a
  fire's write shows up without a command.
- **run-job writes at fire end and after every board act** — the fire
  start write (the fire's read state) stays; the worker refreshes the
  footer after each board act (a fresh wallet + board read, plus the
  act's role, verb, and task id), and the parent writes once more when
  the fire ends.
- **The snapshot gains the last fire** — `lastFire` (role, verb, task
  id, time) rides `status.json`, and the footer's last-memo row shows it:
  `last memo: 3m ago · "claim 7" · worker claim #7 · just now`.
- **Writes are atomic under concurrency** — `WriteSnapshot` uses
  `os.CreateTemp` in the snapshot's directory and renames into place, so
  two fires writing at once never leave a truncated file.

Tests: concurrent snapshot writes leave a parsable file; the status
callback shows a fire's write on the next read.

## [0.2.2] — the wizard splits into moments

`/earn` was one register wizard that needed roles and a goal every run.
It is now a set of moments:

- **bare `/earn`** prints status when the home is set up, otherwise runs
  join with one worker.
- **`/earn join [roles]`** runs the setup (init, vault create, link,
  deposit) then registers the roles (worker by default).
- **`/earn roles`** lists the roster; **`/earn roles add|remove <role>`**
  registers or removes one role and its job.
- **`/earn goal "<text>"`** sets or changes the architect's goal,
  registering an architect (and its job) when none exists.
- **`/earn status|stop|start`** are unchanged.

Before any chain spend the wizard prints one preflight line naming what
exists and what will happen.

The operator key path is asked once, at the first signing step, and
remembered as `ORBIT_OPERATOR_KEY_PATH` in the config — the path, never
the key. A second join after the path is recorded signs with no flag.

Tests: bare `/earn` on a fresh home registers a worker; `roles add
reviewer` creates one job; `goal` on a home with no architect registers
one; a second join after the path is recorded signs with no flag.

## [0.2.1] — the wizard checks before it signs

`/earn` resolved the operator key at the top of the vault create step,
before it checked what needs doing. The wizard now checks the vault
against the recorded creator first and resolves the operator key only at
the step that actually signs (create, link, deposit); a rerun where the
vault, the link and the deposit already exist registers roles with no
key.

The operator configs also stop reading the agent key file: `create` and
the read paths never needed the hot key, so a missing `ORBIT_AGENT_KEY_FILE`
no longer blocks them.

Tests: a wizard run on an existing setup with no `--operator-key-path`
succeeds and sends nothing.

## [0.2.0] — the hygiene pass: nine fixes, each with a test

- **Task rows order numerically** — the board window and render were
  sorted by the task id as text (t1, t10, t2); the window now orders by
  the numeric id.
- **Memo text renders raw — control characters rejected at parse** —
  newlines, tabs, escapes, and the other C0/DEL bytes never become a
  board memo (parse and the write side refuse).
- **resolveMint accepts only real mints** — a 44-character input must
  decode with `sol.Decode` to 32 bytes before it is treated as a mint
  (board and market); an invalid base58 string resolves by FID instead.
- **The legacy env file has the lowest precedence** — it is read into the
  map (live env > env file > config file), never into the process
  (`os.Setenv` is gone).
- **A user-set ORBIT_RPC is used verbatim** — only the indexer-derived
  RPC gets `/rpc` appended; a bare-host ORBIT_RPC no longer gains one.
- **The cron command is shell-quoted** — `RunnerCommand` quotes the
  executable path in the crontab line (spaces and metacharacters are
  safe); all run-job registration sites use it.
- **The job cwd is pinned to the orbit home** — agent jobs no longer
  follow the TUI's cwd.
- **Cost basis is one unit** — the brief and the tools both treat
  `cost_basis_remaining` as lamports (the tools convert to SOL with
  `/1e9`, not `/1e6`).
- **`orbit -update`** — ported from rig's updater: fetches the latest
  `mrsirg97-rgb/orbit` release, verifies `checksums.txt` against the
  pinned minisign key (`ORBIT_UPDATE_KEY` or `settings.json updateKey`;
  unpinned refuses — the rig embedded key is not orbit's), replaces the
  running binary in place, and prints old and new versions.

## [0.1.7] — the fire's brief carries the goal; the sandbox comes from settings

`run-job` built the fire's brief from the agent row's `Name` and `Bio`
only — the architect's `Goal` never appeared in a live brief. The fire
now builds the brief from `Name`, `Bio`, and `Goal`, so the `GOAL:` line
is in every fire's brief.

The sandbox and the worker swap URL came from `ORBIT_SANDBOX` /
`RIG_SWAP_URL` env only, with the swap URL hardcoded to
`http://127.0.0.1:8090` in the run-job path. The fire now reads
`settings.json` the way main does (`config.Load`) and the env overrides:
`ORBIT_SANDBOX` > `settings.json sandbox` > off, `RIG_SWAP_URL` >
`settings.json swapUrl`. No hardcoded swap URL.

Tests: a fire from a row with a goal prints the `GOAL:` line, and a fire
with `settings.json` sandbox on passes the sandbox to the runner (env
overrides both settings keys).

## [0.1.6] — the act waits for its memo to be indexed

`WriteAction` returns at send time and the indexer lags, so the act's
immediate re-sync missed the memo: the reply board omitted the act and
the next act's fold pre-check refused it — a retried task double-spent.

- **Confirm, then wait for the cache** — after the write the act polls
  `GetSignatureStatus` until confirmed (bounded, now the shared
  `client.WaitConfirmed`), then re-syncs until its signature is in the
  cache (bounded, 15s default). If the memo does not land, the reply is
  `<sig> <memo>` plus `pending: not yet indexed` — the write is
  confirmed, only the cache is behind.
- **The reply names a renumbered id** — when the fold renumbers a stale
  task, the reply says which id was assigned, so a follow-up claim does
  not target someone else's task.

Tests: task then claim with the fake's indexer lag 2 succeeds with one
task on chain (no retry double-spend), and the contested-verdict test
now asserts the pending reply for a memo the cache has not seen. The
shared `WaitConfirmed` replaces the project and earn copies.

## [0.1.5] — a board act never guesses the memo's seq

An `Act` inserted its own memo row with `seq = max + 1` and `Sync` never
renumbered it, so a writer's memo ordered before others' and a
`(mint, seq)` collision wedged the project.

- **No local row** — the act writes the memo, then re-syncs: the memo's
  seq is the chain's order, never a local guess.
- **The task id is minted after the sync**, not before, and the fold
  renumbers a task memo whose id is already taken (a stale cache minted
  the same id) to a fresh id in log order.
- **Refuse before spending** — before the write, the act folds the cache
  plus the candidate memo and refuses what the fold would refuse (a
  foreign state, an accept from a non-funder, a stale id) without a
  chain spend.

Tests: two clients folding the same contested verdict agree (both
verdicts land, no `(mint, seq)` wedge), a task from a stale cache gets a
fresh id, and an accept from a non-funder is refused before spending.

## [0.1.4] — the wizard is safe to rerun; the RPC decoders match a real node

The first `/earn` runs after the vault fix found two rerun and wire bugs.

- **earn wizard rerun safety** — deposit is gated on the recorded
  `ORBIT_VAULT_DEPOSITED` marker or the vault record's `total_deposited`
  (never the running balance), link and deposit confirm the signature
  before returning, vault create checks `TorchVaultPDA(creator)` on chain
  and records the creator without sending when the vault exists, a role
  whose job exists is refreshed instead of created, and the model check
  runs before any chain spend.
- **RPC decoders** — `requestAirdrop` returns a bare string signature,
  not `{"value": ...}`; `getSignatureStatuses` uses `confirmationStatus`
  (confirmed/finalized), with `confirmations` null once finalized.
- **The memo cap** — `CurveMemoCap` was 500 and overflowed the 1232-byte
  legacy limit on a curve buy with an ATA. It is now pinned from a
  `sol.Compile` measurement of the worst case (295 bytes, not runes; a
  test re-measures and fails if the builder drifts, so no startup cost),
  and the board and project goal caps count bytes.

## [0.1.3] — the board lease expires only the live claim

The fold's claim case skipped any claim older than the lease
unconditionally, so a task claimed, completed, and accepted in one hour
folded as done today and pending tomorrow. The lease now applies only
while the claim is the task's live state: after the fold, an active task
whose claim is older than the lease returns to pending, and a claim
superseded by complete/accept/reject is never dropped.

## [0.1.2] — the TUI title follows rig's letterforms

The `orbit` art rows didn't match rig's letterforms: `o` had no counter,
`r` and `b` were identical, `i` was two bars, and `t` was a block. The
rows now use rig's letterforms in the same 3-row shape, and the ASCII
fallback name stays `orbit`.

## [0.1.1] — vault create on a clean home

The first real `/earn` run on a clean home found three create-path bugs,
fixed and pinned by tests.

- **wizard vault create** — the create step loaded read mode, so the write
  gate was never on and `SendVaultIx` refused. It now loads the operator
  create config (writes on, no prior creator) and derives the creator from
  the operator key named at the call.
- **`orbit vault create`** — `LoadOperatorConfig` demanded
  `ORBIT_VAULT_CREATOR`, but create is the step that sets it. Create now
  derives the creator from the operator key and needs no prior creator;
  link, deposit, and withdraw keep the requirement.
- **the roles hint** — the "which roles?" error now names `--goal`, which
  an architect requires.

## [0.1.0] — initial release

The torch agent on the rig runtime: the client, the shared board, the
brief, and the scheduled agents.

- **client** — the torch client (SPEC_CLIENT): env-only config loaders
  that fail closed, the embedded IDL (v21.0.0), PDA derivations checked
  against the IDL seeds, the pure quote math, the instruction builders,
  the `RPC` seam (the fake is the test double), the indexer reads, the
  events websocket, the RPC-only scan, and the vault admin path. Reads
  never need a key; the devnet-only gate is structural (`AllowWrite`),
  and the operator's authority key never enters the process.
- **board** — the shared board (SPEC_BOARD): the memo log on the chain
  is the source of truth, the pure fold (memo rows -> tasks + notes) is
  a deterministic projection, the local SQLite is a cache rebuilt from
  the log and never trusted, and the swarm surface
  (claim/note/complete/accept/reject/reap over a `Project`) drains with
  no change to the runtime.
- **brief** — the agent's brief (SPEC_BRIEF): a pure projection of the
  live read side into the compact/full brief, torch's vocabulary (PNL,
  back/exit/post/pass, bonding/ready/migrated/reclaimed,
  HELD/FOUNDED/SENTIMENT), both sizes pinned to the byte by goldens.
- **identity + agent** — one row per (wallet, role) with the role's
  register defaults (cadence, brief size, budget, stall, timeout), and
  one scheduled job per row; each fire rebuilds the brief, refreshes the
  prompt, and runs one-shot.
- **earn** — the `/earn` command (SPEC_EARN): the register wizard (init
  and the vault steps when missing, the operator key named at the call),
  status/stop/start, and the footer snapshot (a local file, never the
  chain).
- **project** — projects (SPEC_PROJECT): `create_token` plus the first
  vault buy that funds the treasury, the `goal:` memo, and `list` (the
  goal per market, the treasury float).
- **onboard** — the one-minute path (ONBOARDING): hot wallet (0600,
  resumable), config upsert, the bounded devnet airdrop, and the
  operator-key seam (flag > env, never stored).
- **tool** — the orbit four on the runtime menu: `market` (buy/sell/post
  via vault + memo), `intel` (the brief's read side), `wallet` (the
  vault read), `board` (the shared board), and the `Snapshot` the brief
  builds from.
- **sol + idl** — the minimal Solana wire (base58, keypairs, legacy
  message compilation, ed25519 signing v0, PDA derivation with the
  on-curve check) and the embedded `torch_market` IDL (v21.0.0, parsed
  once at init, borsh arg encoding). Stdlib only.
- **build + release** — CI (go vet, go test -race -p 2, make fmt-check,
  shellcheck install.sh) and the tagged release workflow: the tag is
  asserted against the `Version` const before any asset is built, the
  four binaries are cross-built with `CGO_ENABLED=0` and checksummed,
  each asset is signed with the pinned minisign key, provenance is
  attested, and the matching CHANGELOG section is the release body. The
  installer verifies the checksum before anything moves.
