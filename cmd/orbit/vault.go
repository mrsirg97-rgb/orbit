package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/onboard"
	"github.com/mrsirg97-rgb/orbit/sol"
)

func runVault(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "orbit: usage: vault create|deposit|withdraw|link|unlink|show [flags]")
		return 2
	}
	action := args[0]
	fs := flag.NewFlagSet("vault-"+action, flag.ContinueOnError)
	opKey := fs.String("operator-key", "", "operator key (base58 64-byte secret)")
	opKeyPath := fs.String("operator-key-path", "", "operator key file path (env: ORBIT_OPERATOR_KEY_PATH)")
	amount := fs.String("sol", "", "SOL amount (deposit/withdraw)")
	wallet := fs.String("wallet", "", "hot wallet pubkey (link/unlink)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	positional := fs.Args()
	if action == "show" {
		return runVaultShow()
	}
	cfg, err := client.LoadOperatorConfig(os.Getenv)
	if err != nil {
		die("vault: %v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("vault: %v", err)
	}
	key, err := onboard.OperatorKey(os.Getenv, *opKey, *opKeyPath)
	if err != nil {
		die("vault: %v", err)
	}
	ctx := context.Background()
	creator := key.PublicBase58()

	switch action {
	case "create":
		ix, err := client.VaultCreateIx(tc.ProgramID, creator, tc.IDL)
		if err != nil {
			die("vault: %v", err)
		}
		sig, err := client.SendVaultIx(ctx, tc, key, "create_vault", []sol.Instruction{ix})
		if err != nil {
			die("vault: %v", err)
		}

		cfgPath, err := onboard.ConfigPath(os.Getenv)
		if err != nil {
			die("vault: %v", err)
		}
		if err := onboard.WriteConfigValue(cfgPath, "ORBIT_VAULT_CREATOR", creator); err != nil {
			die("vault: %v", err)
		}
		fmt.Printf("created vault %s for %s\n", client.TorchVaultPDA(tc.ProgramID, creator), creator)
		fmt.Printf("signature   %s\n", sig)
		fmt.Printf("config      ORBIT_VAULT_CREATOR=%s written to %s\n", creator, cfgPath)
		fmt.Println("next: orbit vault link <hot-wallet-pubkey> && orbit vault deposit 1 && orbit agent register")
	case "deposit", "withdraw":
		amt := *amount
		if amt == "" && len(positional) > 0 {
			amt = positional[0]
		}
		if amt == "" {
			die("vault: SOL amount required (deposit 1)")
		}
		lamports, err := client.ParseSOLAmount(amt)
		if err != nil {
			die("vault: %v", err)
		}
		var ix sol.Instruction
		if action == "deposit" {
			ix, err = client.VaultDepositIx(tc.ProgramID, creator, lamports, tc.IDL)
		} else {
			ix, err = client.VaultWithdrawIx(tc.ProgramID, creator, lamports, tc.IDL)
		}
		if err != nil {
			die("vault: %v", err)
		}
		sig, err := client.SendVaultIx(ctx, tc, key, action+"_vault", []sol.Instruction{ix})
		if err != nil {
			die("vault: %v", err)
		}
		fmt.Printf("%s %s SOL -> %s\n", action, amt, client.TorchVaultPDA(tc.ProgramID, creator))
		fmt.Printf("signature %s\n", sig)
	case "link", "unlink":
		hot := *wallet
		if hot == "" && len(positional) > 0 {
			hot = positional[0]
		}
		if hot == "" {
			die("vault: hot wallet pubkey required (link <pubkey>)")
		}
		var ix sol.Instruction
		if action == "link" {
			ix, err = client.VaultLinkIx(tc.ProgramID, creator, hot, tc.IDL)
		} else {
			ix, err = client.VaultUnlinkIx(tc.ProgramID, creator, hot, tc.IDL)
		}
		if err != nil {
			die("vault: %v", err)
		}
		sig, err := client.SendVaultIx(ctx, tc, key, action+"_wallet", []sol.Instruction{ix})
		if err != nil {
			die("vault: %v", err)
		}
		fmt.Printf("%s %s %s\n", action, hot, client.VaultWalletLinkPDA(tc.ProgramID, hot))
		fmt.Printf("signature %s\n", sig)
	default:
		fmt.Fprintf(os.Stderr, "orbit: vault: unknown action %q\n", action)
		return 2
	}
	return 0
}

func runVaultShow() int {
	cfg, err := client.LoadOperatorConfig(os.Getenv)
	if err != nil {
		die("vault: %v", err)
	}
	tc, err := client.New(cfg)
	if err != nil {
		die("vault: %v", err)
	}
	show, err := client.ShowVault(context.Background(), tc, cfg.VaultCreator)
	if err != nil {
		die("vault: %v", err)
	}
	fmt.Printf("VAULT     %s\n", show.Vault)
	fmt.Printf("CREATOR   %s\n", show.Creator)
	fmt.Printf("AUTHORITY %s\n", show.Authority)
	fmt.Printf("SOL       %s\n", client.FormatSOL(show.VaultSOL))
	fmt.Printf("LINKED    %d wallet(s)\n", show.LinkedWallets)
	fmt.Printf("DEPOSITED %s SOL\n", client.FormatSOL(show.TotalDeposited))
	fmt.Printf("WITHDRAWN %s SOL\n", client.FormatSOL(show.TotalWithdrawn))
	fmt.Printf("SPENT     %s SOL\n", client.FormatSOL(show.TotalSpent))
	fmt.Printf("RECEIVED  %s SOL\n", client.FormatSOL(show.TotalReceived))
	return 0
}
