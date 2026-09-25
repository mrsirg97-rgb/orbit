package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/mrsirg97-rgb/orbit/brief"
	"github.com/mrsirg97-rgb/orbit/client"
	orbittool "github.com/mrsirg97-rgb/orbit/tool"
	"os"
)

func runSnapshot(args []string) int {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	full := fs.Bool("full", false, "the full block (default compact)")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg, err := client.LoadConfig(os.Getenv)
	if err != nil {
		die("%v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("%v", err)
	}
	snap, err := orbittool.Snapshot(context.Background(), tc, brief.Identity{Name: "@AP" + walletSuffix(tc.AgentPublic()), Bio: "torch agent"})
	if err != nil {
		die("snapshot: %v", err)
	}
	size := brief.Compact
	if *full {
		size = brief.Full
	}
	block, err := brief.Build(snap, size)
	if err != nil {
		die("snapshot: %v", err)
	}
	fmt.Print(block)
	fmt.Fprintf(os.Stderr, "orbit: %d tokens (%d bytes)\n", brief.Tokens(block), len(block))
	return 0
}
