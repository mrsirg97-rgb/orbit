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

- **`List` / `Filter`**: `project.List` rows filtered by status
  (bonding|ready|migrated) or goal-only, sorted by treasury descending —
  the economic signal that decides what a worker joins.
- **`Show`**: one project by full mint or 8-char FID — goal, treasury,
  board summary (n/m done, open claims), and the last three memos.
- **`Command`**: the `/projects` slash command — `/projects list
  [bonding|ready|migrated|goal]`, `/projects show <mint|fid>` — offering
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
- The open-task count and the board summary fold the newest 50 messages
  per market (the project list's window): a task log deeper than the
  window counts only what it folded.

## Gotchas

- The indexer serves newest-first; the fold sorts the messages into log
  order (ascending `message_id`) before `board.Fold`, so claim/complete/
  accept apply in order.
- The goal is the first `goal:` memo in the indexer's order (the project
  list's convention — the newest goal is the current one).
- Reclaimed projects show in the default list but take no status filter
  (the filter vocabulary is bonding|ready|migrated).
