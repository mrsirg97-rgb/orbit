package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/client"
)

// Board is the shared-board tool: read a project's board, or act — task,
// brief, claim, note, complete, accept, reject. Every act is one memo + one
// vault-routed micro buy; the reply carries the tx signature plus the memo.
// No roles: any wallet may act — the fold decides ownership (the funder's
// accept) and stake.
type Board struct {
	Store  *board.Store
	Client func() (*client.TorchClient, error)
}

func (b *Board) client() (*client.TorchClient, error) {
	if b.Client == nil {
		return nil, fmt.Errorf("board: no client seam (run /earn)")
	}
	tc, err := b.Client()
	if err != nil {
		return nil, fmt.Errorf("board: %w", err)
	}
	return tc, nil
}

func (b *Board) Name() string { return "board" }

func (b *Board) Description() string {
	return "the shared board: read a project's board (goal, tasks, claims, verdicts); act: task, brief, claim, note, complete, accept, reject. Every act is a memo plus a vault-routed micro buy; the reply is the tx signature plus the memo."
}

func (b *Board) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"mint": {"type": "string", "description": "Full mint pubkey or 8-char FID (RPC-only needs the full mint)."},
			"action": {"type": "string", "enum": ["task", "brief", "claim", "note", "complete", "accept", "reject"], "description": "Omit to read only."},
			"id": {"type": "integer", "description": "The task id (the memo's id); the task action mints the next id itself."},
			"text": {"type": "string", "description": "The memo text: task/brief/note/reject text; omit for claim/complete/accept."}
		},
		"required": ["mint"]
	}`)
}

func (b *Board) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	var in struct {
		Mint   string `json:"mint"`
		Action string `json:"action"`
		ID     int    `json:"id"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("board: args: %w", err)
	}
	in.Mint = strings.TrimSpace(in.Mint)
	if in.Mint == "" {
		return "", fmt.Errorf("board: mint required")
	}
	mint, err := b.resolveMint(ctx, in.Mint)
	if err != nil {
		return "", fmt.Errorf("board: %w", err)
	}
	project := board.Project{Mint: mint, Label: b.label(ctx, mint)}
	if in.Action == "" {
		return b.Store.Board(ctx, project)
	}
	shape := board.Shape{Verb: in.Action, ID: in.ID, Text: in.Text}
	if in.Action == "task" && in.ID == 0 {
		next, err := b.Store.NextID(ctx, project)
		if err != nil {
			return "", fmt.Errorf("board: task id: %w", err)
		}
		shape.ID = next
	}
	if shape.ID == 0 {
		return "", fmt.Errorf("board: %s needs a task id", in.Action)
	}
	return b.Store.Act(ctx, project, shape)
}

// resolveMint: a full mint is validated by the store's market read; an
// 8-char FID resolves through the indexer's market list (RPC-only boards
// need the full mint).
func (b *Board) resolveMint(ctx context.Context, input string) (string, error) {
	tc, err := b.client()
	if err != nil {
		return "", err
	}
	if len(input) == 44 {
		if _, err := b.Store.Market(ctx, input); err == nil {
			return input, nil
		}
		return "", fmt.Errorf("no project with mint %s", input)
	}
	if tc.Indexer == "" {
		return "", fmt.Errorf("RPC-only boards need the full mint (ORBIT_INDEXER unset)")
	}
	markets, err := tc.API.Markets(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("resolve %s: %w", input, err)
	}
	for _, m := range markets {
		if fid8(m.Mint) == input {
			return m.Mint, nil
		}
	}
	return "", fmt.Errorf("no project with FID %s", input)
}

func (b *Board) label(ctx context.Context, mint string) string {
	tc, err := b.client()
	if err != nil {
		return ""
	}
	if tc.Indexer == "" {
		return ""
	}
	m, err := tc.API.Market(ctx, mint)
	if err != nil {
		return ""
	}
	return m.Market.Name
}
