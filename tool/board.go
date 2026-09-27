package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/client"
	solpkg "github.com/mrsirg97-rgb/orbit/sol"
)

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
	return "the shared board: read a project's board (goal, tasks, claims, verdicts); act: task, brief, note, complete, accept, reject. A claim is capital: pick up a task with contract or work on the market tool, let it go with release. A task needs no id (minted after the sync); work verbs land only on a public project; an act the fold would refuse is refused before spending; only the funder's accept counts. The reply is the signature, the memo, and the board, or \"pending: not yet indexed\", or the assigned-id line when the fold renumbers."
}

func (b *Board) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"mint": {"type": "string", "description": "Full mint pubkey or 8-char FID (RPC-only needs the full mint)."},
			"action": {"type": "string", "enum": ["task", "brief", "note", "complete", "accept", "reject"], "description": "Omit to read only. To pick up a task use the market tool: contract or work."},
			"id": {"type": "integer", "description": "The memo's id; omit for task (the id is minted after the sync)."},
			"text": {"type": "string", "description": "The memo text: task/brief/note/reject text; omit for complete/accept."}
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
	if in.Action == "claim" || in.Action == "release" {
		return "", fmt.Errorf("board: %s is capital: use the market tool (contract, work, release)", in.Action)
	}
	shape := board.Shape{Verb: in.Action, ID: in.ID, Text: in.Text}
	if shape.ID == 0 && in.Action != "task" {
		return "", fmt.Errorf("board: %s needs a task id", in.Action)
	}
	return b.Store.Act(ctx, project, shape)
}

func (b *Board) resolveMint(ctx context.Context, input string) (string, error) {
	tc, err := b.client()
	if err != nil {
		return "", err
	}
	if decoded, err := solpkg.Decode(input); err == nil && len(decoded) == 32 {
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
