package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/onboard"
	"github.com/mrsirg97-rgb/orbit/project"
)

// runProject is the operator's project surface: create a market with a goal
// (create_token + first vault buy), list what the indexer knows. The operator
// key comes from a flag or env per call and is never stored.
func runProject(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "orbit: usage: project create|list [flags]")
		return 2
	}
	switch args[0] {
	case "create":
		return runProjectCreate(args[1:])
	case "list":
		return runProjectList()
	default:
		fmt.Fprintf(os.Stderr, "orbit: project: unknown action %q\n", args[0])
		return 2
	}
}

func runProjectCreate(args []string) int {
	fs := flag.NewFlagSet("project-create", flag.ContinueOnError)
	name := fs.String("name", "", "project name")
	goal := fs.String("goal", "", "one-paragraph goal (the memo on the first buy)")
	treasury := fs.String("treasury", "", "SOL amount of the first buy (funds the treasury)")
	opKey := fs.String("operator-key", "", "operator key (base58 64-byte secret)")
	opKeyPath := fs.String("operator-key-path", "", "operator key file path (env: ORBIT_OPERATOR_KEY_PATH)")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *name == "" {
		die("project: -name required")
	}
	if *goal == "" {
		die("project: -goal required")
	}
	lamports, err := client.ParseSOLAmount(*treasury)
	if err != nil {
		die("project: -treasury: %v", err)
	}
	if lamports == 0 {
		die("project: -treasury must be > 0")
	}
	cfg, err := client.LoadConfig(os.Getenv)
	if err != nil {
		die("project: %v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("project: %v", err)
	}
	key, err := onboard.OperatorKey(os.Getenv, *opKey, *opKeyPath)
	if err != nil {
		die("project: %v", err)
	}
	res, err := project.Create(context.Background(), tc, key, *name, *goal, lamports)
	if err != nil {
		die("project: %v", err)
	}
	fmt.Printf("PROJECT  %s (%s)\n", res.Name, res.Symbol)
	fmt.Printf("MINT     %s\n", res.Mint)
	fmt.Printf("FID      %s\n", fid8(res.Mint))
	fmt.Printf("GOAL     %s\n", res.Goal)
	fmt.Printf("TREASURY %s SOL (first buy)\n", client.FormatSOL(res.TreasuryLamports))
	fmt.Printf("CREATE   %s\n", res.CreateSignature)
	fmt.Printf("BUY      %s\n", res.BuySignature)
	return 0
}

func runProjectList() int {
	cfg, err := client.LoadConfig(os.Getenv)
	if err != nil {
		die("project: %v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("project: %v", err)
	}
	rows, err := project.List(context.Background(), tc.API, tc.RPC, tc.ProgramID, 50)
	if err != nil {
		die("project: %v", err)
	}
	fmt.Printf("%-10s %-20s %-8s %-10s %-10s %s\n", "FID", "NAME", "SYMBOL", "STATUS", "TREASURY", "GOAL")
	for _, r := range rows {
		goal := r.Goal
		if goal == "" {
			goal = "-"
		}
		fmt.Printf("%-10s %-20s %-8s %-10s %-10s %s\n",
			fid8(r.Mint), truncate(r.Name, 20), r.Symbol, r.Status,
			client.FormatSOL(r.TreasurySOL), goal)
	}
	return 0
}

func fid8(mint string) string {
	if len(mint) <= 8 {
		return mint
	}
	return mint[len(mint)-8:]
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}
