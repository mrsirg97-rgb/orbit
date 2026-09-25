package client

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/mrsirg97-rgb/orbit/sol"
)

const vaultTestProgram = "FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh"

func wantDisc(t *testing.T, name string) []byte {
	t.Helper()
	d, err := loadIDL(t).Discriminator(name)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestVaultBuildersAgainstIDL(t *testing.T) {
	id := loadIDL(t)
	const operator = "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	const hot = "4we9cx8o8YoLR9QxyhKiUGrMx2QyCrH8rkpnGXqQwktm"

	create, err := VaultCreateIx(vaultTestProgram, operator, id)
	if err != nil {
		t.Fatal(err)
	}
	assertIx(t, create, "create_vault", wantDisc(t, "create_vault"), nil, []string{
		operator, TorchVaultPDA(vaultTestProgram, operator), VaultSolPDA(vaultTestProgram, operator),
		VaultWalletLinkPDA(vaultTestProgram, operator), SystemProgram,
	}, []bool{true, false, false, false, false}, []bool{true, true, true, true, false})

	deposit, err := VaultDepositIx(vaultTestProgram, operator, 500_000_000, id)
	if err != nil {
		t.Fatal(err)
	}
	assertIx(t, deposit, "deposit_vault", wantDisc(t, "deposit_vault"), uint64(500_000_000), []string{
		operator, TorchVaultPDA(vaultTestProgram, operator), VaultSolPDA(vaultTestProgram, operator), SystemProgram,
	}, []bool{true, false, false, false}, []bool{true, true, true, false})

	withdraw, err := VaultWithdrawIx(vaultTestProgram, operator, 1_000_000_000, id)
	if err != nil {
		t.Fatal(err)
	}
	assertIx(t, withdraw, "withdraw_vault", wantDisc(t, "withdraw_vault"), uint64(1_000_000_000), []string{
		operator, TorchVaultPDA(vaultTestProgram, operator), VaultSolPDA(vaultTestProgram, operator), SystemProgram,
	}, []bool{true, false, false, false}, []bool{true, true, true, false})

	link, err := VaultLinkIx(vaultTestProgram, operator, hot, id)
	if err != nil {
		t.Fatal(err)
	}
	assertIx(t, link, "link_wallet", wantDisc(t, "link_wallet"), nil, []string{
		operator, TorchVaultPDA(vaultTestProgram, operator), hot,
		VaultWalletLinkPDA(vaultTestProgram, hot), SystemProgram,
	}, []bool{true, false, false, false, false}, []bool{true, true, false, true, false})

	unlink, err := VaultUnlinkIx(vaultTestProgram, operator, hot, id)
	if err != nil {
		t.Fatal(err)
	}
	assertIx(t, unlink, "unlink_wallet", wantDisc(t, "unlink_wallet"), nil, []string{
		operator, TorchVaultPDA(vaultTestProgram, operator), hot,
		VaultWalletLinkPDA(vaultTestProgram, hot), SystemProgram,
	}, []bool{true, false, false, false, false}, []bool{true, true, false, true, false})
}

// assertIx checks the discriminator, the borsh arg payload, and the exact
// account order/flags against the IDL (positional anchor contexts).
func assertIx(t *testing.T, ix sol.Instruction, name string, disc []byte, arg any, keys []string, signers, writable []bool) {
	t.Helper()
	if got := ix.Data[:len(disc)]; string(got) != string(disc) {
		t.Errorf("%s discriminator %x, want %x", name, got, disc)
	}
	if arg != nil {
		u64 := arg.(uint64)
		want := make([]byte, 8)
		binary.LittleEndian.PutUint64(want, u64)
		if got := ix.Data[len(disc):]; string(got) != string(want) {
			t.Errorf("%s args %x, want %x", name, got, want)
		}
	} else if len(ix.Data) != len(disc) {
		t.Errorf("%s data %x, want only the discriminator %x", name, ix.Data, disc)
	}
	if len(ix.Accounts) != len(keys) {
		t.Fatalf("%s accounts %d, want %d", name, len(ix.Accounts), len(keys))
	}
	for i, k := range keys {
		if ix.Accounts[i].Pubkey != k {
			t.Errorf("%s account %d %s, want %s", name, i, ix.Accounts[i].Pubkey, k)
		}
		if ix.Accounts[i].IsSigner != signers[i] {
			t.Errorf("%s account %d signer %v, want %v", name, i, ix.Accounts[i].IsSigner, signers[i])
		}
		if ix.Accounts[i].IsWritable != writable[i] {
			t.Errorf("%s account %d writable %v, want %v", name, i, ix.Accounts[i].IsWritable, writable[i])
		}
	}
}

func TestParseSOLAmount(t *testing.T) {
	for in, want := range map[string]uint64{
		"1": 1_000_000_000, "1.5": 1_500_000_000, "0.25": 250_000_000,
		"0.000000001": 1, "1.000000000": 1_000_000_000,
	} {
		got, err := ParseSOLAmount(in)
		if err != nil || got != want {
			t.Errorf("ParseSOLAmount(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	if _, err := ParseSOLAmount(""); err == nil {
		t.Error("empty amount should error")
	}
	if _, err := ParseSOLAmount("abc"); err == nil {
		t.Error("bad amount should error")
	}
}

func TestFormatSOL(t *testing.T) {
	cases := map[uint64]string{
		1_000_000_000: "1.000000000",
		2_500_000:     "0.002500000",
		1:             "0.000000001",
		0:             "0.000000000",
		1_250_000_000: "1.250000000",
	}
	for in, want := range cases {
		if got := FormatSOL(in); got != want {
			t.Errorf("FormatSOL(%d) = %s, want %s", in, got, want)
		}
	}
}

func TestDecodeTorchVault(t *testing.T) {
	data := make([]byte, 114)
	copy(data[:8], []byte{0x8c, 0x41, 0x81, 0x38, 0x97, 0xba, 0x02, 0x47})
	creator := "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	cb, _ := sol.Decode(creator)
	copy(data[8:40], cb)
	copy(data[40:72], cb)
	binary.LittleEndian.PutUint64(data[72:80], 1_500_000_000)
	binary.LittleEndian.PutUint64(data[88:96], 250_000_000)
	data[104] = 2
	binary.LittleEndian.PutUint64(data[105:113], 1_700_000_000)
	data[113] = 253
	rec, err := DecodeTorchVault(data)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Creator != creator || rec.Authority != creator {
		t.Errorf("creator/authority %s/%s", rec.Creator, rec.Authority)
	}
	if rec.TotalDeposited != 1_500_000_000 || rec.TotalSpent != 250_000_000 {
		t.Errorf("totals %+v", rec)
	}
	if rec.LinkedWallets != 2 || rec.CreatedAt != 1_700_000_000 || rec.Bump != 253 {
		t.Errorf("record %+v", rec)
	}
}

type fakeVaultRPC struct {
	accounts map[string]AccountInfo
}

func (f *fakeVaultRPC) GetAccountInfo(ctx context.Context, pubkey string) (AccountInfo, error) {
	if a, ok := f.accounts[pubkey]; ok {
		return a, nil
	}
	return AccountInfo{}, nil
}
func (f *fakeVaultRPC) GetLatestBlockhash(context.Context) (string, error) {
	return "5TbMPrxpUmSW8KEdqDA4jYotCR7XjoQkoFH5VzA5ayzt", nil
}
func (f *fakeVaultRPC) SendTransaction(context.Context, []byte) (string, error) { return "sig", nil }
func (f *fakeVaultRPC) GetTokenAccountsByOwner(context.Context, string, string) ([]TokenAccount, error) {
	return nil, nil
}
func (f *fakeVaultRPC) GetBalance(context.Context, string) (uint64, error) { return 0, nil }
func (f *fakeVaultRPC) GetSignatureStatus(context.Context, string) (SignatureStatus, error) {
	return SignatureStatus{}, nil
}
func (f *fakeVaultRPC) RequestAirdrop(context.Context, string, uint64) (string, error) {
	return "", nil
}
func (f *fakeVaultRPC) GetSignaturesForAddress(context.Context, string, int) ([]SignatureInfo, error) {
	return nil, nil
}
func (f *fakeVaultRPC) GetTransaction(context.Context, string) (*Transaction, error) {
	return nil, nil
}

func TestShowVaultAgainstFixture(t *testing.T) {
	creator := "8GQ4XGM9p5DqKjw2JTrUAc42adwYWD5PK3P7eTobcYKy"
	data := make([]byte, 114)
	cb, _ := sol.Decode(creator)
	copy(data[8:40], cb)
	copy(data[40:72], cb)
	binary.LittleEndian.PutUint64(data[72:80], 3_000_000_000)
	data[104] = 3
	fake := &fakeVaultRPC{accounts: map[string]AccountInfo{
		TorchVaultPDA(vaultTestProgram, creator): {Exists: true, Data: data, Owner: vaultTestProgram},
		VaultSolPDA(vaultTestProgram, creator):   {Exists: true, Lamports: 2_000_000_000 + RentExemptZeroData},
	}}
	tc := &TorchClient{Config: Config{ProgramID: vaultTestProgram}, RPC: fake}
	show, err := ShowVault(context.Background(), tc, creator)
	if err != nil {
		t.Fatal(err)
	}
	if show.Vault != TorchVaultPDA(vaultTestProgram, creator) {
		t.Errorf("vault %s", show.Vault)
	}
	if show.VaultSOL != 2_000_000_000 {
		t.Errorf("vault sol %d", show.VaultSOL)
	}
	if show.LinkedWallets != 3 || show.TotalDeposited != 3_000_000_000 {
		t.Errorf("show %+v", show)
	}
}
