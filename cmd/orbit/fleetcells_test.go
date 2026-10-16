package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mrsirg97-rgb/rig/v3/tool"
	"github.com/mrsirg97-rgb/rig/v3/tool/delegate"
)

type cellDelegate struct {
	tool.Definition
	command string
	submit  string
}

func (d *cellDelegate) Exec(ctx context.Context, args json.RawMessage) (string, error) {
	return "", nil
}

func (d *cellDelegate) Run(ctx context.Context, task, workspace, model string) (string, error) {
	return "", nil
}

func (d *cellDelegate) Command(ctx context.Context, command, workspace string, bound time.Duration) (string, error) {
	return d.command, nil
}

func (d *cellDelegate) Submit(ctx context.Context, kind, name string, bound time.Duration, work func(ctx context.Context) (string, int)) (string, error) {
	return d.submit, nil
}

func (d *cellDelegate) StopAll() {}

func TestTheBashCellRefusesADeferredCommandWhenEmpty(t *testing.T) {
	var cell delegate.Delegate
	w := &fleetBash{&cell}
	out, err := w.Command(context.Background(), "sleep 5", "/tmp", time.Minute)
	if err == nil || !strings.Contains(err.Error(), "no worker") {
		t.Fatalf("an empty cell refuses loudly, got %q %v", out, err)
	}
	cell = &cellDelegate{Definition: tool.Def("delegate"), command: "done"}
	if out, err := w.Command(context.Background(), "sleep 5", "/tmp", time.Minute); err != nil || out != "done" {
		t.Fatalf("a filled cell forwards, got %q %v", out, err)
	}
}

func TestThePythonCellRefusesADeferredCellWhenEmpty(t *testing.T) {
	var cell delegate.Delegate
	w := &fleetPython{&cell}
	out, err := w.Submit(context.Background(), "kernel", "name", time.Minute, func(ctx context.Context) (string, int) { return "", 0 })
	if err == nil || !strings.Contains(err.Error(), "no worker") {
		t.Fatalf("an empty cell refuses loudly, got %q %v", out, err)
	}
	cell = &cellDelegate{Definition: tool.Def("delegate"), submit: "ran"}
	if out, err := w.Submit(context.Background(), "kernel", "name", time.Minute, func(ctx context.Context) (string, int) { return "", 0 }); err != nil || out != "ran" {
		t.Fatalf("a filled cell forwards, got %q %v", out, err)
	}
}
