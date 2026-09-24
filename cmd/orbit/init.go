package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/onboard"
)

// runInit is `orbit init`: hot wallet + config + devnet airdrop, one minute.
func runInit(args []string) int {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	force := fs.Bool("force", false, "overwrite an existing key")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	res, err := onboard.Init(onboard.InitOpts{
		Getenv: os.Getenv,
		Force:  *force,
		// The airdrop goes to the direct devnet faucet (the indexer proxy
		// rate-limits requestAirdrop); reads/writes still use the config RPC.
		RPC: client.NewJSONRPC(client.DevnetAirdropRPC),
	})
	if err != nil {
		die("init: %v", err)
	}
	fmt.Printf("AGENT WALLET  %s\n", res.Pubkey)
	fmt.Printf("BALANCE       %s SOL\n", client.FormatSOL(res.Balance))
	fmt.Printf("KEY           %s (0600; the only copy)\n", res.KeyPath)
	fmt.Printf("CONFIG        %s\n", res.ConfigPath)
	fmt.Println("NEXT (operator key is never stored here):")
	for _, line := range res.Next {
		fmt.Printf("  %s\n", line)
	}
	return 0
}
