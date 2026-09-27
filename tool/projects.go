package tool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/board"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/projects"
)

type Projects struct {
	Client func() (*client.TorchClient, error)
	Store  *board.Store
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
	return "where an agent picks a project before it contracts: list the projects with goal, state, backing, and open tasks (status=private|funded|public, goal-only), sorted by backing; show <project> the goal, backing, board summary (n/m done, open claims), and the last three memos. Read-only, keyless."
}

func (p *Projects) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"action": {"type": "string", "enum": ["list", "show"], "description": "Omit to list."},
			"status": {"type": "string", "enum": ["private", "funded", "public"], "description": "List filter: the project state."},
			"goal": {"type": "boolean", "description": "List filter: only projects with a goal."},
			"project": {"type": "string", "description": "show: the PID (8 chars) or the full project mint."}
		}
	}`)
}

func (p *Projects) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	tc, err := p.client()
	if err != nil {
		return "", err
	}
	var in struct {
		Action  string `json:"action"`
		Status  string `json:"status"`
		Goal    bool   `json:"goal"`
		Project string `json:"project"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("projects: args: %w", err)
	}
	switch in.Action {
	case "", "list":
		return p.list(ctx, tc, projects.Filter{Status: strings.TrimSpace(in.Status), GoalOnly: in.Goal})
	case "show":
		in.Project = strings.TrimSpace(in.Project)
		if in.Project == "" {
			return "", errors.New("projects: show needs a project (PID or mint)")
		}
		s, err := projects.Show(ctx, tc, p.Store, in.Project)
		if err != nil {
			return "", fmt.Errorf("projects: %w", err)
		}
		return projects.ShowText(s), nil
	default:
		return "", fmt.Errorf("projects: unknown action %q (list, show)", in.Action)
	}
}

func (p *Projects) list(ctx context.Context, tc *client.TorchClient, f projects.Filter) (string, error) {
	rows, err := projects.List(ctx, tc, p.Store, f)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "projects: none match", nil
	}
	lines := make([]string, 0, len(rows))
	for _, r := range rows {
		lines = append(lines, fmt.Sprintf("[%s] %s · %s · backing %s SOL · %s open · goal %s",
			pid8(r.Mint), r.Name, brief.StateWord(r.Status), client.FormatSOL(r.TreasurySOL),
			projects.TasksText(r.OpenTasks), dash(r.Goal)))
	}
	return strings.Join(lines, "\n"), nil
}

func dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
