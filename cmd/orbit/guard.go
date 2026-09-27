package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/mrsirg97-rgb/rig/core"
)

// keyGuard refuses a tool call whose arguments name the agent's hot key.
// The fire worker signs with that key, so the file sits inside the jail's
// grant and the kernel cannot hide it; this guard is the tool layer saying
// no to the accidental read (a model that logs the key path and then
// looks). It is not a wall against a determined model with a shell; that
// wall is signing outside the worker, a later change.
type keyGuard struct {
	core.Tool
	paths []string
}

var errKeyGuarded = errors.New("permission denied: the agent's key is not readable by tools")

func guardKey(t core.Tool, paths ...string) core.Tool {
	clean := make([]string, 0, len(paths))
	for _, p := range paths {
		if p = strings.TrimSpace(p); p != "" {
			clean = append(clean, filepath.Clean(p))
		}
	}
	if len(clean) == 0 {
		return t
	}
	return &keyGuard{Tool: t, paths: clean}
}

func (g *keyGuard) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	if namesKey(args, g.paths) {
		return "", errKeyGuarded
	}
	return g.Tool.Exec(ctx, args)
}

// namesKey walks every string in the arguments (paths, commands, code)
// and reports one that mentions a key path, by the path as given or by its
// last two elements ("<home>/key"), so a relative spelling does not slip.
func namesKey(args json.RawMessage, paths []string) bool {
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return false
	}
	var found bool
	var walk func(any)
	walk = func(x any) {
		if found {
			return
		}
		switch t := x.(type) {
		case string:
			for _, p := range paths {
				if strings.Contains(t, p) {
					found = true
					return
				}
				tail := filepath.Join(filepath.Base(filepath.Dir(p)), filepath.Base(p))
				if strings.Contains(t, tail) {
					found = true
					return
				}
			}
		case []any:
			for _, e := range t {
				walk(e)
			}
		case map[string]any:
			for _, e := range t {
				walk(e)
			}
		}
	}
	walk(v)
	return found
}
