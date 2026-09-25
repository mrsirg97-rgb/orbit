package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrsirg97-rgb/orbit/client"
)

type Intel struct {
	Client func() (*client.TorchClient, error)
}

func (t *Intel) client() (*client.TorchClient, error) {
	if t.Client == nil {
		return nil, fmt.Errorf("intel: no client seam (run /earn)")
	}
	tc, err := t.Client()
	if err != nil {
		return nil, fmt.Errorf("intel: %w", err)
	}
	return tc, nil
}

func (t *Intel) Name() string { return "intel" }

func (t *Intel) Description() string {
	return "recent messages on held/watched projects: sender, memo, slot. Read-only."
}

func (t *Intel) Schema() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"mint": {"type": "string", "description": "Full mint pubkey or 8-char FID. Omit for all held/watched."},
			"limit": {"type": "integer", "minimum": 1, "maximum": 100, "description": "Default 10."}
		}
	}`)
}

func (t *Intel) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	tc, err := t.client()
	if err != nil {
		return "", err
	}
	var in struct {
		Mint  string `json:"mint"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return "", fmt.Errorf("intel: args: %w", err)
	}
	if in.Limit <= 0 {
		in.Limit = 10
	}
	if in.Limit > 100 {
		in.Limit = 100
	}
	mints := []string{}
	if in.Mint != "" {
		full, err := resolveMint(ctx, tc, in.Mint)
		if err != nil {
			return "", fmt.Errorf("intel: %w", err)
		}
		mints = append(mints, full)
	} else {
		markets, err := tc.API.Markets(ctx, nil)
		if err != nil {
			return "", fmt.Errorf("intel: markets: %w", err)
		}
		for _, m := range markets {
			mints = append(mints, m.Mint)
		}
	}
	var lines []string
	for _, mint := range mints {
		msgs, err := tc.API.Messages(ctx, client.Q("mint", mint, "limit", fmt.Sprint(in.Limit)))
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			lines = append(lines, fmt.Sprintf("%s %s: %s",
				fid8(msg.Mint), shortAddr(msg.Sender), truncate(msg.MemoText, 160)))
		}
	}
	if len(lines) == 0 {
		return "no recent intel", nil
	}
	return strings.Join(lines, "\n"), nil
}

func shortAddr(s string) string {
	if len(s) <= 8 {
		return s
	}
	return s[:4] + "…" + s[len(s)-4:]
}
