package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/projects"
)

type Projects struct {
	Client func() (*client.TorchClient, error)
}

func (p *Projects) client() (*client.TorchClient, error) {
	if p.Client == nil {
		return nil, errors.New("projects: no read seam (run /earn)")
	}
	tc, err := p.Client()
	if err != nil {
		return nil, fmt.Errorf("projects: %w", err)
	}
	return tc, nil
}

func (p *Projects) Name() string { return "projects" }

func (p *Projects) Description() string {
	return "where an agent picks a project before it claims: list the projects with goal, status, treasury, and open tasks (status=bonding|ready|migrated, goal-only), sorted by treasury; show <mint|fid> the goal, treasury, board summary (n/m done, open claims), and the last three memos. Read-only, keyless."
}

func (p *Projects) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["list", "show"], "description": "Omit to list."},
			"status": {"type": "string", "enum": ["bonding", "ready", "migrated"], "description": "List filter."},
			"goal": {"type": "boolean", "description": "List filter: only projects with a goal."},
			"mint": {"type": "string", "description": "Full mint pubkey or 8-char FID (show)."}
		}
	}`)
}

func (p *Projects) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	tc, err := p.client()
	if err != nil {
		return "", err
	}
	var in struct {
		Action string `json:"action"`
		Status string `json:"status"`
		Goal   bool   `json:"goal"`
		Mint   string `json:"mint"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("projects: args: %w", err)
	}
	switch in.Action {
	case "", "list":
		return p.list(ctx, tc, projects.Filter{Status: strings.TrimSpace(in.Status), GoalOnly: in.Goal})
	case "show":
		in.Mint = strings.TrimSpace(in.Mint)
		if in.Mint == "" {
			return "", errors.New("projects: show needs a mint or FID")
		}
		s, err := projects.Show(ctx, tc, in.Mint)
		if err != nil {
			return "", fmt.Errorf("projects: %w", err)
		}
		return projects.ShowText(s), nil
	default:
		return "", fmt.Errorf("projects: unknown action %q (list, show)", in.Action)
	}
}

func (p *Projects) list(ctx context.Context, tc *client.TorchClient, f projects.Filter) (string, error) {
	rows, err := projects.List(ctx, tc, f)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "projects: none match", nil
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("[%s] %s · %s · treasury %s SOL · %d open · goal %s",
			fid8(r.Mint), r.Name, projects.StatusWord(r.Status), client.FormatSOL(r.TreasurySOL),
			r.OpenTasks, dash(r.Goal)))
	}
	return strings.Join(lines, "\n"), nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
