package client

import (
	"bytes"
	"context"
	"encoding/binary"
	"net/url"
	"testing"

	"github.com/mrsirg97-rgb/orbit/sol"
)

// fakeRPC records the signed transaction and answers canned reads.
type fakeRPC struct {
	sent      string
	blockhash string
	accounts  map[string]AccountInfo
	balance   uint64
}

func (f *fakeRPC) GetLatestBlockhash(ctx context.Context) (string, error) {
	return f.blockhash, nil
}

func (f *fakeRPC) SendTransaction(ctx context.Context, signed []byte) (string, error) {
	f.sent = sol.EncodeTx(signed)
	return "sigFAKE1234567890", nil
}

func (f *fakeRPC) GetAccountInfo(ctx context.Context, pubkey string) (AccountInfo, error) {
	return f.accounts[pubkey], nil
}

func (f *fakeRPC) GetTokenAccountsByOwner(ctx context.Context, owner, programID string) ([]TokenAccount, error) {
	return nil, nil
}

func (f *fakeRPC) GetBalance(ctx context.Context, pubkey string) (uint64, error) {
	return f.balance, nil
}

func (f *fakeRPC) GetSignatureStatus(ctx context.Context, signature string) (SignatureStatus, error) {
	return SignatureStatus{Exists: true, Confirmed: true}, nil
}

// txParts decodes a signed legacy transaction: signature, header, keys,
// blockhash, instructions.
type txParts struct {
	Sig  []byte
	Keys []string
	Ixs  []parsedIx
}

type parsedIx struct {
	ProgramID string
	Accounts  []int
	Data      []byte
}

func parseTx(t *testing.T, signed string) txParts {
	t.Helper()
	raw, err := sol.Decode(signed)
	if err != nil {
		t.Fatal(err)
	}
	// The client sends the v0 versioned wire:
	// [0x01 count][64-byte sig][0x80][legacy message][0x00].
	parts := txParts{Sig: raw[1:65]}
	b := raw[65:]
	if b[0] == 0x80 {
		b = b[1:]
		b = b[:len(b)-1]
	}
	b = b[3:]
	nKeys, b := readCompact(t, b)
	for i := 0; i < nKeys; i++ {
		parts.Keys = append(parts.Keys, sol.Encode(b[:32]))
		b = b[32:]
	}
	b = b[32:] // blockhash
	nIxs, b := readCompact(t, b)
	for i := 0; i < nIxs; i++ {
		prog := int(b[0])
		b = b[1:]
		acctN := int(b[0])
		b = b[1:]
		accts := make([]int, acctN)
		for j := 0; j < acctN; j++ {
			accts[j] = int(b[0])
			b = b[1:]
		}
		dataLen, rest := readCompact(t, b)
		data := rest[:dataLen]
		b = rest[dataLen:]
		parts.Ixs = append(parts.Ixs, parsedIx{ProgramID: parts.Keys[prog], Accounts: accts, Data: data})
	}
	return parts
}

func readCompact(t *testing.T, b []byte) (int, []byte) {
	t.Helper()
	v := int(b[0])
	if v&0x80 != 0 {
		v = (v & 0x7f) | int(b[1])<<7
		return v, b[2:]
	}
	return v, b[1:]
}

func testClient(t *testing.T, rpc RPC) *TorchClient {
	t.Helper()
	cfg := Config{
		Indexer:      "http://127.0.0.1:1",
		RPC:          "http://127.0.0.1:1",
		ProgramID:    DevnetProgramID,
		VaultCreator: "So11111111111111111111111111111111111111112",
		AllowWrite:   true,
	}
	seed := make([]byte, 32)
	copy(seed, "orbit-write-test-seed-0000000")
	raw := make([]byte, 64)
	copy(raw[:32], seed)
	copy(raw[32:], solPub(seed))
	kp, err := sol.KeypairFromSecret(sol.Encode(raw))
	if err != nil {
		t.Fatal(err)
	}
	cfg.AgentKey = kp
	tc, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tc.API = &stubAPI{market: MarketRow{
		Mint:    "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG",
		Creator: "FghCwWojts9MbU3Pmog5peacaKrEYM5n1T68KWHy7TAh",
		Status:  StatusBonding, VirtualSol: 150_000_000_000, VirtualToken: 1_000_000_000_000_000,
		RealSol: 50_000_000_000, SolTarget: 200_000_000_000,
	}}
	tc.RPC = rpc
	return tc
}

func solPub(seed []byte) []byte {
	pub, err := sol.PublicFromSeed(seed)
	if err != nil {
		panic(err)
	}
	return pub
}

type stubAPI struct {
	market MarketRow
}

func (s *stubAPI) Markets(ctx context.Context, q url.Values) ([]MarketRow, error) { return nil, nil }
func (s *stubAPI) Market(ctx context.Context, mint string) (MarketDetail, error) {
	return MarketDetail{Market: s.market}, nil
}
func (s *stubAPI) Messages(ctx context.Context, q url.Values) ([]MessageRow, error) { return nil, nil }
func (s *stubAPI) Trades(ctx context.Context, q url.Values) ([]TradeRow, error)     { return nil, nil }
func (s *stubAPI) Positions(ctx context.Context, q url.Values) ([]PositionRow, error) {
	return nil, nil
}
func (s *stubAPI) PositionEvents(ctx context.Context, q url.Values) ([]PositionEventRow, error) {
	return nil, nil
}
func (s *stubAPI) Migrations(ctx context.Context, q url.Values) ([]MigrationRow, error) {
	return nil, nil
}
func (s *stubAPI) Pnl(ctx context.Context, wallet, vault string) (PnlSummary, error) {
	return PnlSummary{}, nil
}
func (s *stubAPI) Swaps(ctx context.Context, q url.Values) ([]SwapRow, error) { return nil, nil }

func globalConfigAccount(t *testing.T) AccountInfo {
	t.Helper()
	data := make([]byte, 8+32+32+32+2+8+8+1)
	// authority (32 zero bytes), treasury (32 zero bytes), dev_wallet (WSOL),
	// protocol_fee_bps 50, total_tokens_launched, total_volume, bump.
	dev, err := sol.Decode("So11111111111111111111111111111111111111112")
	if err != nil {
		t.Fatal(err)
	}
	copy(data[8+64:8+64+32], dev)
	data[8+96] = 50 // protocol_fee_bps (u16 LE)
	data[8+97] = 0
	return AccountInfo{Exists: true, Data: data}
}

func TestWriteBackSignsAndMemos(t *testing.T) {
	rpc := &fakeRPC{blockhash: "11111111111111111111111111111111"}
	rpc.accounts = map[string]AccountInfo{GlobalConfigPDA(DevnetProgramID): globalConfigAccount(t)}
	tc := testClient(t, rpc)
	res, err := tc.WriteAction(context.Background(), tc.API.(*stubAPI).market, ActionBack, "backed up strong", 10_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if res.Signature != "sigFAKE1234567890" {
		t.Errorf("signature: %s", res.Signature)
	}
	if res.Memo != "backed up strong" {
		t.Errorf("memo: %s", res.Memo)
	}
	parts := parseTx(t, rpc.sent)
	// The payer is the agent hot wallet.
	if parts.Keys[0] != tc.AgentPublic() {
		t.Errorf("payer: %s", parts.Keys[0])
	}
	// The torch buy instruction carries the IDL discriminator + borsh args.
	var buyIx *parsedIx
	var memoIx *parsedIx
	for i := range parts.Ixs {
		if parts.Ixs[i].ProgramID == DevnetProgramID {
			buyIx = &parts.Ixs[i]
		}
		if parts.Ixs[i].ProgramID == MemoProgram {
			memoIx = &parts.Ixs[i]
		}
	}
	if buyIx == nil {
		t.Fatal("no torch instruction in the tx")
	}
	if !bytes.Equal(buyIx.Data[:8], []byte{213, 46, 240, 54, 205, 19, 39, 25}) {
		t.Errorf("buy discriminator: %x", buyIx.Data[:8])
	}
	if got := binary.LittleEndian.Uint64(buyIx.Data[8:16]); got != 10_000_000 {
		t.Errorf("sol_amount: %d", got)
	}
	if got := binary.LittleEndian.Uint64(buyIx.Data[16:24]); got != 56_352_542_130 {
		t.Errorf("min_tokens_out: %d", got)
	}
	if memoIx == nil {
		t.Fatal("no memo instruction")
	}
	if string(memoIx.Data) != "backed up strong" {
		t.Errorf("memo data: %q", string(memoIx.Data))
	}
	// The memo signer account is the hot wallet.
	if parts.Keys[memoIx.Accounts[0]] != tc.AgentPublic() {
		t.Errorf("memo signer: %s", parts.Keys[memoIx.Accounts[0]])
	}
}

func TestWriteCutSellsWholeVaultHolding(t *testing.T) {
	rpc := &fakeRPC{blockhash: "11111111111111111111111111111111"}
	tc := testClient(t, rpc)
	vaultATA, _ := tc.ATAFor(tc.API.(*stubAPI).market.Mint)
	tokenData := make([]byte, 165)
	binary.LittleEndian.PutUint64(tokenData[64:72], 123_456_789)
	rpc.accounts = map[string]AccountInfo{
		vaultATA: {Exists: true, Data: tokenData},
	}
	res, err := tc.WriteAction(context.Background(), tc.API.(*stubAPI).market, ActionCut, "cutting weak", 0)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "sell" {
		t.Errorf("kind: %s", res.Kind)
	}
	parts := parseTx(t, rpc.sent)
	var sellIx *parsedIx
	for i := range parts.Ixs {
		if parts.Ixs[i].ProgramID == DevnetProgramID {
			sellIx = &parts.Ixs[i]
		}
	}
	if sellIx == nil {
		t.Fatal("no sell instruction")
	}
	if got := binary.LittleEndian.Uint64(sellIx.Data[8:16]); got != 123_456_789 {
		t.Errorf("token_amount: %d", got)
	}
}

func TestWriteRefusesNonDevnet(t *testing.T) {
	cfg := Config{
		Indexer: "http://127.0.0.1:1", RPC: "http://127.0.0.1:1",
		ProgramID:    "NonDevnetProgramId1111111111111111111111111111",
		VaultCreator: "So11111111111111111111111111111111111111112",
		AllowWrite:   false,
	}
	if _, err := New(cfg); err == nil {
		t.Fatal("non-devnet config accepted")
	}
}
