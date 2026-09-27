# projects

## What it is

The discovery surface: the `projects` tool and the `/projects` command
that list what the indexer knows — goal, status, treasury, open task
count — and show one project's goal, treasury, board summary, and the
last three memos. Read-only and keyless: the reads are the project
list's read path (markets, the message log, the RPC treasury float) and
the board's pure fold; no chain writes, no agent key. SPEC_PROJECT's
discovery section governs.

## What it includes

- **`List` / `Filter`**: the discovery list, filtered by status
  (private|funded|public) or goal-only and sorted by backing
  descending — the economic signal that decides what a worker joins. The
  count is cache-first: a project the board cache has synced is read
  from the walked log (goal + open tasks), an uncached one falls back to
  the newest-50 window, and the fallback is bounded (`FallbackBudget` =
  10 window fetches per list, given to the highest-treasury uncached
  projects). A project beyond the budget shows unknown (`-`).
- **`Show`**: one project by 8-char PID or full mint — goal, backing,
  board summary (n/m done, open claims), and the last three memos. When
  the board cache has the project, the goal and the summary come from
  the walked log; the memos always come from the window (the newest
  three are never outside it).
- **`Command`**: the `/projects` slash command — `/projects list
  [private|funded|public|goal]`, `/projects show <pid|mint>` — offering
  the two verbs through `command.Subber`; the list prints as a table, one
  row per project.
- **`ShowText`**: the shared show block (goal, treasury, board summary,
  memos) used by the tool and the command.

## How it is consumed

- Both the tool and the command take the client through the lazy read
  seam (`client.LoadReadConfig` + `NewRead`): indexer + RPC, no vault
  creator, no agent key. A missing config fails loudly naming `/earn`.
- A bare token that is not a filter resolves as a show target, so
  `/projects <fid>` works; `list` and `show` are the picker's verbs.
- The board cache is the window the count trusts: a cached project's
  count is the walked log (up to the 50-page bound), an uncached one is
  the newest-50 window, and the fallback fan-out is bounded so the list
  stays one markets call plus a handful of message calls as projects
  grow.

## Gotchas

- The indexer serves newest-first; the fold sorts the messages into log
  order (ascending `message_id`) before `board.Fold`, so claim/complete/
  accept apply in order.
- The goal's source differs by row: a cached project's goal is the first
  `goal:` memo in the walked log (the board read's goal), an uncached
  one's is the first in the indexer's order (the newest).
- Reclaimed projects show in the default list but take no status filter
  (the filter vocabulary is the state words private|funded|public, one
  source: `brief.StateWord`).
