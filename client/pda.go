package client

import (
	"fmt"

	"github.com/mrsirg97-rgb/orbit/sol"
)

// Program IDs and seeds (mirror programs/torch_market/src/constants.rs and
// packages/sdk/src/constants.ts).
const (
	DevnetProgramID   = "FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh"
	DefaultIndexer    = "https://api.torchmarket.dev"
	DefaultRPC        = "https://api.torchmarket.dev"
	DevnetAirdropRPC  = "https://api.devnet.solana.com"
	DeepPoolProgramID = "CcwF61GW14AcxCS4E2zedHXdFXy8x8GQPvfxZrs2x2eT"
	Token2022Program  = "TokenzQdBNbLqP5VEhdkAS6EPFLC1PHnBqCXEpPxuEb"
	ATProgram         = "ATokenGPvbdGVxr1b2hvZbsiqW5xWH25efTNsLJA8knL"
	MemoProgram       = "MemoSq4gqABAXKb96qnH8TysNcWxMyWCqXgDLGmfcHr"
	SystemProgram     = "11111111111111111111111111111111"
)

var (
	seedGlobalConfig     = []byte("global_config")
	seedBondingCurve     = []byte("bonding_curve")
	seedBondingCurveSol  = []byte("bonding_curve_sol")
	seedTreasury         = []byte("treasury")
	seedTreasurySolVault = []byte("treasury_sol_vault")
	seedUserPosition     = []byte("user_position")
	seedUserStats        = []byte("user_stats")
	seedProtocolTreasury = []byte("protocol_treasury_v11")
	seedTorchVault       = []byte("torch_vault")
	seedTorchVaultSol    = []byte("torch_vault_sol")
	seedVaultWallet      = []byte("vault_wallet")
	seedTorchConfig      = []byte("torch_config")
	seedDeepPool         = []byte("deep_pool")
	seedPoolVault        = []byte("pool_vault")
	seedEventAuthority   = []byte("__event_authority")
)

func mustPDA(program string, seeds ...[]byte) string {
	pk, _, err := sol.PDA(program, seeds...)
	if err != nil {
		panic(fmt.Sprintf("pda: %v", err))
	}
	return pk
}

// GlobalConfigPDA is the protocol singleton.
func GlobalConfigPDA(program string) string { return mustPDA(program, seedGlobalConfig) }

// BondingCurvePDA is per-mint curve state.
func BondingCurvePDA(program, mint string) string {
	return mustPDA(program, seedBondingCurve, must32(mint))
}

// BondingCurveSolPDA is the system-owned curve SOL custody.
func BondingCurveSolPDA(program, mint string) string {
	return mustPDA(program, seedBondingCurveSol, must32(mint))
}

// TokenTreasuryPDA is per-mint treasury accounting.
func TokenTreasuryPDA(program, mint string) string {
	return mustPDA(program, seedTreasury, must32(mint))
}

// TreasurySolVaultPDA is the system-owned per-mint treasury SOL custody.
func TreasurySolVaultPDA(program, mint string) string {
	return mustPDA(program, seedTreasurySolVault, must32(mint))
}

// ProtocolTreasuryPDA is the rewards singleton.
func ProtocolTreasuryPDA(program string) string { return mustPDA(program, seedProtocolTreasury) }

// TorchVaultPDA is the per-creator vault.
func TorchVaultPDA(program, creator string) string {
	return mustPDA(program, seedTorchVault, must32(creator))
}

// VaultSolPDA is the system-owned vault SOL home.
func VaultSolPDA(program, creator string) string {
	return mustPDA(program, seedTorchVaultSol, must32(creator))
}

// VaultWalletLinkPDA is the reverse pointer from a wallet to its vault.
func VaultWalletLinkPDA(program, wallet string) string {
	return mustPDA(program, seedVaultWallet, must32(wallet))
}

// UserPositionPDA tracks per-(curve, user) buy history.
func UserPositionPDA(program, bondingCurve, user string) string {
	return mustPDA(program, seedUserPosition, must32(bondingCurve), must32(user))
}

// UserStatsPDA tracks per-user volume.
func UserStatsPDA(program, user string) string { return mustPDA(program, seedUserStats, must32(user)) }

// TorchConfigPDA is the torch-owned namespace for DeepPool pools.
func TorchConfigPDA(program string) string { return mustPDA(program, seedTorchConfig) }

// DeepPoolPDA is the pool for a mint under the torch config namespace.
func DeepPoolPDA(program, torchConfig, mint string) string {
	return mustPDA(program, seedDeepPool, must32(torchConfig), must32(mint))
}

// DeepPoolVaultPDA is the pool's token vault.
func DeepPoolVaultPDA(program, pool string) string {
	return mustPDA(program, seedPoolVault, must32(pool))
}

// DeepPoolEventAuthorityPDA is deep_pool's emit_cpi authority.
func DeepPoolEventAuthorityPDA(program string) string { return mustPDA(program, seedEventAuthority) }

// TorchEventAuthorityPDA is torch's emit_cpi authority (the IDL's
// `event_authority` account on every #[event_cpi] context).
func TorchEventAuthorityPDA(program string) string { return mustPDA(program, seedEventAuthority) }

// ATA derives the associated token account for owner under tokenProgram.
// Seed order is owner || tokenProgram || mint (the SPL ATA convention,
// see @solana/spl-token state/mint.js).
func ATA(mint, owner, tokenProgram string) (string, error) {
	ata, _, err := sol.PDA(ATProgram, must32(owner), must32(tokenProgram), must32(mint))
	return ata, err
}

func must32(s string) []byte {
	b, err := sol.Decode(s)
	if err != nil {
		panic(fmt.Sprintf("pda: bad pubkey %q: %v", s, err))
	}
	return b
}
