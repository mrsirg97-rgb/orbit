package projects

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/rig/command"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/project"
)

type Command struct {
	Client func() (*client.TorchClient, error)
}

func (c *Command) Name() string { return "projects" }

func (c *Command) Description() string {
	return "where an agent picks a project before it claims: /projects list [bonding|ready|migrated|goal], /projects show <mint|fid>"
}

func (c *Command) Sub() []command.Sub {
	return []command.Sub{
		{Name: "list", Desc: "list [bonding|ready|migrated|goal]: the projects table, sorted by treasury"},
		{Name: "show", Desc: "show <mint|fid>: the goal, treasury, board summary (n/m done, open claims), and the last three memos"},
	}
}

func (c *Command) Run(ctx context.Context, args string, env any) (string, error) {
	tc, err := c.client(ctx)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(args)
	if len(fields) == 0 {
		return c.list(ctx, tc, Filter{})
	}
	switch fields[0] {
	case "list":
		f, err := parseFilter(fields[1:])
		if err != nil {
			return "", err
		}
		return c.list(ctx, tc, f)
	case "show":
		if len(fields) != 2 {
			return "", errors.New("projects: show needs a mint or FID (projects show <mint|fid>)")
		}
		return c.show(ctx, tc, fields[1])
	}
	if len(fields) != 1 {
		return "", errors.New("projects: one filter at a time (bonding|ready|migrated|goal) or a mint/FID")
	}
	if isFilter(fields[0]) {
		return c.list(ctx, tc, filterFor(fields[0]))
	}
	return c.show(ctx, tc, fields[0])
}

func (c *Command) client(ctx context.Context) (*client.TorchClient, error) {
	if c.Client == nil {
		return nil, errors.New("projects: no read seam (run /earn)")
	}
	tc, err := c.Client()
	if err != nil {
		return nil, fmt.Errorf("projects: %w", err)
	}
	return tc, nil
}

func (c *Command) list(ctx context.Context, tc *client.TorchClient, f Filter) (string, error) {
	rows, err := List(ctx, tc, f)
	if err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "projects: none match", nil
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("%-10s %-20s %-9s %-14s %-4s %s\n", "FID", "NAME", "STATUS", "TREASURY", "OPEN", "GOAL"))
	for _, r := range rows {
		b.WriteString(rowLine(r))
	}
	return b.String(), nil
}

func (c *Command) show(ctx context.Context, tc *client.TorchClient, input string) (string, error) {
	s, err := Show(ctx, tc, input)
	if err != nil {
		return "", err
	}
	return ShowText(s), nil
}

func rowLine(r project.Row) string {
	return fmt.Sprintf("%-10s %-20s %-9s %-14s %-4d %s\n",
		fid8(r.Mint), truncate(r.Name, 20), StatusWord(r.Status), client.FormatSOL(r.TreasurySOL),
		r.OpenTasks, dash(r.Goal))
}

func parseFilter(fields []string) (Filter, error) {
	if len(fields) == 0 {
		return Filter{}, nil
	}
	if len(fields) > 1 {
		return Filter{}, errors.New("projects: one filter at a time (bonding|ready|migrated|goal)")
	}
	if !isFilter(fields[0]) {
		return Filter{}, fmt.Errorf("projects: unknown filter %q (bonding|ready|migrated|goal)", fields[0])
	}
	return filterFor(fields[0]), nil
}

func isFilter(s string) bool {
	switch s {
	case "bonding", "ready", "migrated", "goal":
		return true
	}
	return false
}

func filterFor(s string) Filter {
	if s == "goal" {
		return Filter{GoalOnly: true}
	}
	return Filter{Status: s}
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
