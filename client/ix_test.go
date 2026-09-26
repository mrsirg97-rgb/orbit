package client

import (
	"encoding/hex"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/orbit/idl"
	"github.com/mrsirg97-rgb/orbit/sol"
)

func loadIDL(t *testing.T) *idl.IDL {
	t.Helper()
	id, err := idl.LoadIDL()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestBuyViaVaultGolden(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	wantDisc := "d52ef036cd132719"
	if hex.EncodeToString(disc) != wantDisc {
		t.Fatalf("discriminator %x, want %s", disc, wantDisc)
	}
	args, err := id.BorshArgs("buy_via_vault", map[string]any{
		"sol_amount":     uint64(10_000_000),
		"min_tokens_out": uint64(9_900_000),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := hex.EncodeToString(append(append([]byte{}, disc...), args...))
	want := "d52ef036cd132719" + "8096980000000000" + "e00f970000000000"
	if got != want {
		t.Fatalf("data %s, want %s", got, want)
	}
}

func TestSellViaVaultGolden(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("sell_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	wantDisc := "ce4753215361e23b"
	if hex.EncodeToString(disc) != wantDisc {
		t.Fatalf("discriminator %x, want %s", disc, wantDisc)
	}
	args, err := id.BorshArgs("sell_via_vault", map[string]any{
		"token_amount": uint64(1_000_000_000),
		"min_sol_out":  uint64(148_024),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := hex.EncodeToString(append(append([]byte{}, disc...), args...))
	want := "ce4753215361e23b" + "00ca9a3b00000000" + "3842020000000000"
	if got != want {
		t.Fatalf("data %s, want %s", got, want)
	}
}

func TestVaultSwapGolden(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("vault_swap")
	if err != nil {
		t.Fatal(err)
	}
	wantDisc := "8fc29a8ae6deed1c"
	if hex.EncodeToString(disc) != wantDisc {
		t.Fatalf("discriminator %x, want %s", disc, wantDisc)
	}

	data := append(append([]byte{}, disc...), leU64(10_000_000)...)
	data = append(data, leU64(9_000_000)...)
	data = append(data, 1)
	want := "8fc29a8ae6deed1c" + "8096980000000000" + "4054890000000000" + "01"
	if hex.EncodeToString(data) != want {
		t.Fatalf("data %x, want %s", data, want)
	}
}

func TestBuyAccountOrderAndPDAs(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	const buyer = "So11111111111111111111111111111111111111112"
	const creator = "FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh"
	ixs, err := BuildBuyViaVault(disc, BuyAccounts{
		Mint: mint, Creator: creator, DevWallet: buyer,
		Buyer: buyer, VaultCreator: buyer, ProgramID: DevnetProgramID, WithATA: true,
	}, 10_000_000, 9_900_000, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(ixs) != 2 {
		t.Fatalf("got %d instructions, want 2", len(ixs))
	}
	buy := ixs[1]
	if buy.ProgramID != DevnetProgramID {
		t.Errorf("program: %s", buy.ProgramID)
	}
	names, err := id.AccountNames("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	if len(buy.Accounts) != len(names) {
		t.Fatalf("accounts %d, IDL names %d", len(buy.Accounts), len(names))
	}
	wantFirst := []string{"buyer", "global_config", "dev_wallet", "mint", "bonding_curve"}
	for i, want := range wantFirst {
		if buy.Accounts[i].Pubkey == "" {
			t.Fatalf("account %d empty", i)
		}
		_ = want
	}
	if !buy.Accounts[0].IsSigner || !buy.Accounts[0].IsWritable {
		t.Error("buyer must be signer + writable")
	}

	bc := BondingCurvePDA(DevnetProgramID, mint)
	if bc != "6wDUn9V7fuP1Ujn6o3xk4yFh4EE3F65LpgQsrNjTjmVx" {
		t.Errorf("bonding curve PDA: %s", bc)
	}
	if buy.Accounts[4].Pubkey != bc {
		t.Errorf("bonding_curve account: %s", buy.Accounts[4].Pubkey)
	}
	vault := TorchVaultPDA(DevnetProgramID, buyer)
	if vault != "HkPCW8mFF6ndNGzKufyfsYLcmpjTaGVKaoDAjwjBauh4" {
		t.Errorf("vault PDA: %s", vault)
	}
	if buy.Accounts[14].Pubkey != vault {
		t.Errorf("torch_vault account: %s", buy.Accounts[14].Pubkey)
	}
	vaultATA, err := ATA(mint, vault, Token2022Program)
	if err != nil {
		t.Fatal(err)
	}
	if vaultATA != "2xYXam7tjfmLHu2HDNQzx2aiiTFp24Nie5Bv5y3jTmSp" {
		t.Errorf("vault ATA: %s", vaultATA)
	}
	if buy.Accounts[17].Pubkey != vaultATA {
		t.Errorf("vault_token_account: %s", buy.Accounts[17].Pubkey)
	}

	if ixs[0].ProgramID != ATProgram {
		t.Errorf("ATA program: %s", ixs[0].ProgramID)
	}
	if len(ixs[0].Data) != 1 || ixs[0].Data[0] != 1 {
		t.Errorf("ATA data: %x", ixs[0].Data)
	}
}

func TestVaultSwapAccountOrder(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("vault_swap")
	if err != nil {
		t.Fatal(err)
	}
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	const signer = "So11111111111111111111111111111111111111112"
	ixs, err := BuildVaultSwap(disc, SwapAccounts{
		Mint: mint, Signer: signer, VaultCreator: signer,
		ProgramID: DevnetProgramID, DeepPoolID: DeepPoolProgramID,
	}, 10_000_000, 9_000_000, true, "memo", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(ixs) != 3 {
		t.Fatalf("got %d instructions, want 3", len(ixs))
	}
	swap := ixs[1]
	if !swap.Accounts[0].IsSigner {
		t.Error("signer must be signer")
	}
	pool := DeepPoolPDA(DeepPoolProgramID, TorchConfigPDA(DevnetProgramID), mint)
	if pool != "H5uVsoK2KydBthZYgMhh5Z2mJBRNfr7ZCZpLiVb9zjCf" {
		t.Errorf("deep pool PDA: %s", pool)
	}
	if swap.Accounts[8].Pubkey != pool {
		t.Errorf("deep_pool account: %s", swap.Accounts[8].Pubkey)
	}
	memo := ixs[2]
	if memo.ProgramID != MemoProgram {
		t.Errorf("memo program: %s", memo.ProgramID)
	}
	if string(memo.Data) != "memo" {
		t.Errorf("memo data: %q", string(memo.Data))
	}
}

func TestMemoCap(t *testing.T) {
	long := make([]rune, 501)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := BuildMemo("wallet", string(long)); err != nil {
		t.Fatalf("memo builder must not cap (the callers enforce the path cap): %v", err)
	}
}

func TestMemoAtCapCompilesUnderLegacyLimit(t *testing.T) {
	// The drift guard for the pinned CurveMemoCap: the worst case is a buy
	// whose wallet pubkeys (buyer, creator, dev wallet, vault creator) are
	// all distinct, so the message cannot collapse keys.
	id := loadIDL(t)
	disc, err := id.Discriminator("buy_via_vault")
	if err != nil {
		t.Fatal(err)
	}
	kp, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	creator, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	devWallet, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	vaultCreator, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	size := func(memo string) int {
		ixs, err := BuildBuyViaVault(disc, BuyAccounts{
			Mint: mint, Creator: creator.PublicBase58(), DevWallet: devWallet.PublicBase58(),
			Buyer: kp.PublicBase58(), VaultCreator: vaultCreator.PublicBase58(), ProgramID: DevnetProgramID,
			WithATA: true,
		}, MemoBuyLamports, 1, memo)
		if err != nil {
			t.Fatal(err)
		}
		msg, err := sol.Compile("11111111111111111111111111111111", kp.PublicBase58(), ixs)
		if err != nil {
			t.Fatal(err)
		}
		return len(sol.SignVersionedTx(msg, kp))
	}
	if got := size(strings.Repeat("x", CurveMemoCap)); got > legacyTxLimit {
		t.Errorf("memo at the cap: tx %d bytes, want <= %d", got, legacyTxLimit)
	}
	if got := size(strings.Repeat("x", CurveMemoCap+1)); got <= legacyTxLimit {
		t.Errorf("memo over the cap: tx %d bytes, want > %d", got, legacyTxLimit)
	}
}

func TestCreateTokenGolden(t *testing.T) {
	id := loadIDL(t)
	disc, err := id.Discriminator("create_token")
	if err != nil {
		t.Fatal(err)
	}
	wantDisc := "5434cce4188cea4b"
	if hex.EncodeToString(disc) != wantDisc {
		t.Fatalf("discriminator %x, want %s", disc, wantDisc)
	}
	args, err := id.BorshArgs("create_token", map[string]any{
		"name":            "Context Compaction",
		"symbol":          "CONTEX",
		"uri":             "",
		"sol_target":      uint64(200_000_000_000),
		"community_token": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := hex.EncodeToString(append(append([]byte{}, disc...), args...))
	want := wantDisc +
		"12000000" + "436f6e7465787420436f6d70616374696f6e" +
		"06000000" + "434f4e544558" +
		"00000000" +
		"00d0ed902e000000" +
		"00"
	if got != want {
		t.Fatalf("data %s, want %s", got, want)
	}
}

func TestCreateTokenAccountOrderAndPDAs(t *testing.T) {
	id := loadIDL(t)
	const mint = "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	const operator = "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	ix, err := BuildCreateToken(DevnetProgramID, CreateTokenAccounts{
		Creator: operator, Mint: mint, ProgramID: DevnetProgramID,
	}, CreateTokenArgs{Name: "Context Compaction", Symbol: "CONTEX", URI: "", SolTarget: 200_000_000_000, CommunityToken: false}, id)
	if err != nil {
		t.Fatal(err)
	}
	names, err := id.AccountNames("create_token")
	if err != nil {
		t.Fatal(err)
	}
	if len(ix.Accounts) != len(names) || len(names) != 16 {
		t.Fatalf("accounts %d, IDL names %d (want 16)", len(ix.Accounts), len(names))
	}
	bc := BondingCurvePDA(DevnetProgramID, mint)
	treasury := TokenTreasuryPDA(DevnetProgramID, mint)
	treasurySol := TreasurySolVaultPDA(DevnetProgramID, mint)
	lock := TreasuryLockPDA(DevnetProgramID, mint)
	bcATA, err := ATA(mint, bc, Token2022Program)
	if err != nil {
		t.Fatal(err)
	}
	treasuryATA, err := ATA(mint, treasury, Token2022Program)
	if err != nil {
		t.Fatal(err)
	}
	lockATA, err := ATA(mint, lock, Token2022Program)
	if err != nil {
		t.Fatal(err)
	}
	wantKeys := []string{
		operator, GlobalConfigPDA(DevnetProgramID), mint, bc, bcATA,
		treasury, treasurySol, treasuryATA, lock, lockATA,
		Token2022Program, ATProgram, SystemProgram, RentProgram,
		TorchEventAuthorityPDA(DevnetProgramID), DevnetProgramID,
	}
	for i, want := range wantKeys {
		if ix.Accounts[i].Pubkey != want {
			t.Errorf("account %d %s, want %s", i, ix.Accounts[i].Pubkey, want)
		}
	}
	if !ix.Accounts[0].IsSigner || !ix.Accounts[0].IsWritable {
		t.Error("creator must be signer + writable")
	}
	if !ix.Accounts[2].IsSigner || !ix.Accounts[2].IsWritable {
		t.Error("mint must be signer + writable")
	}
	if !ix.Accounts[3].IsWritable || !ix.Accounts[4].IsWritable ||
		!ix.Accounts[5].IsWritable || !ix.Accounts[6].IsWritable ||
		!ix.Accounts[7].IsWritable || !ix.Accounts[8].IsWritable ||
		!ix.Accounts[9].IsWritable {
		t.Error("curve/treasury accounts must be writable")
	}

	if bc != "6wDUn9V7fuP1Ujn6o3xk4yFh4EE3F65LpgQsrNjTjmVx" {
		t.Errorf("bonding curve PDA: %s", bc)
	}
	if treasury != "6qYP4kqANocDpaiMNu5eVijMhRVzXEpL4FzDmPreRTcX" {
		t.Errorf("treasury PDA: %s", treasury)
	}
	if treasurySol != "B9f9i5pgEgvvRgQzzyQ98tVYy3KKc9z3qavu4xVp5nDy" {
		t.Errorf("treasury sol PDA: %s", treasurySol)
	}
	if lock != "6195P6Z7fNWNf2xyVRWkjdGtcRhsQhbfAL6hsQM1Th52" {
		t.Errorf("treasury lock PDA: %s", lock)
	}
	if bcATA != "Dhffw9mKgRiCr5tA56Y4R8hoaVeZDKThpVuYDSG7kRhs" {
		t.Errorf("bonding curve ATA: %s", bcATA)
	}
	if treasuryATA != "GZYt7j3XxyE5vsy66ctAW8UVn3Gj1iWVaETZD5RjG5Tx" {
		t.Errorf("treasury ATA: %s", treasuryATA)
	}
	if lockATA != "GQDmuVmMfaooXTcayDsLS5QXbb1MjkvnbKjJUW2r9ZVv" {
		t.Errorf("treasury lock ATA: %s", lockATA)
	}
}
