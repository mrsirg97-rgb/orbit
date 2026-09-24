package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/mrsirg97-rgb/orbit/sol"
)

// RentExemptZeroData is minimum_balance(0) — the lamport floor for the
// System-owned 0-data PDAs (vault_sol, treasury_sol_vault, bonding_curve_sol).
const RentExemptZeroData uint64 = 890880

// AccountInfo is one getAccountInfo result.
type AccountInfo struct {
	Lamports uint64
	Owner    string
	Data     []byte
	Exists   bool
}

// TokenAccount is one parsed token account from getTokenAccountsByOwner.
type TokenAccount struct {
	Pubkey   string
	Mint     string
	Amount   uint64
	Owner    string
	IsNative bool
}

// SignatureStatus is one getSignatureStatus result.
type SignatureStatus struct {
	Exists        bool
	Confirmed     bool
	Err           string
	Confirmations *uint64
}

// RPC is the single seam to the chain. The fake is the test double.
type RPC interface {
	GetLatestBlockhash(ctx context.Context) (string, error)
	SendTransaction(ctx context.Context, signedRaw []byte) (string, error)
	GetAccountInfo(ctx context.Context, pubkey string) (AccountInfo, error)
	GetTokenAccountsByOwner(ctx context.Context, owner, programID string) ([]TokenAccount, error)
	GetBalance(ctx context.Context, pubkey string) (uint64, error)
	GetSignatureStatus(ctx context.Context, signature string) (SignatureStatus, error)
	// RequestAirdrop funds a wallet on devnet (the proxy forwards it).
	RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error)
}

// JSONRPC implements RPC over POST {base}/rpc (the indexer's passthrough or
// a direct node).
type JSONRPC struct {
	base string
	cli  *http.Client
	id   atomic.Int64
}

// NewJSONRPC returns an RPC backed by the base URL.
func NewJSONRPC(base string) *JSONRPC {
	return &JSONRPC{base: strings.TrimSuffix(base, "/"), cli: &http.Client{Timeout: 30 * time.Second}}
}

type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      int64         `json:"id"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (r *JSONRPC) call(ctx context.Context, method string, params ...interface{}) (json.RawMessage, error) {
	id := r.id.Add(1)
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base+"/rpc", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json")
	resp, err := r.cli.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rpc %s: %w", method, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rpc %s: status %d: %s", method, resp.StatusCode, string(raw))
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *rpcError       `json:"error"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("rpc %s: decode: %w", method, err)
	}
	if envelope.Error != nil {
		return nil, fmt.Errorf("rpc %s: %d %s", method, envelope.Error.Code, envelope.Error.Message)
	}
	return envelope.Result, nil
}

func (r *JSONRPC) GetLatestBlockhash(ctx context.Context) (string, error) {
	raw, err := r.call(ctx, "getLatestBlockhash")
	if err != nil {
		return "", err
	}
	// The node returns {"value": {"blockhash": ...}}; tolerate the flat form.
	var out struct {
		Value struct {
			Blockhash string `json:"blockhash"`
		} `json:"value"`
		Blockhash string `json:"blockhash"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("rpc getLatestBlockhash: %w", err)
	}
	bh := out.Value.Blockhash
	if bh == "" {
		bh = out.Blockhash
	}
	if bh == "" {
		return "", fmt.Errorf("rpc getLatestBlockhash: empty blockhash")
	}
	return bh, nil
}

func (r *JSONRPC) SendTransaction(ctx context.Context, signedRaw []byte) (string, error) {
	raw, err := r.call(ctx, "sendTransaction", base64.StdEncoding.EncodeToString(signedRaw),
		map[string]interface{}{"encoding": "base64", "preflightCommitment": "confirmed"})
	if err != nil {
		return "", err
	}
	var sig string
	if err := json.Unmarshal(raw, &sig); err != nil {
		return "", fmt.Errorf("rpc sendTransaction: %w", err)
	}
	return sig, nil
}

func (r *JSONRPC) GetAccountInfo(ctx context.Context, pubkey string) (AccountInfo, error) {
	raw, err := r.call(ctx, "getAccountInfo", pubkey,
		map[string]interface{}{"encoding": "base64"})
	if err != nil {
		return AccountInfo{}, err
	}
	var out struct {
		Value *struct {
			Lamports uint64  `json:"lamports"`
			Owner    string  `json:"owner"`
			Data     []any   `json:"data"`
			Space    *uint64 `json:"space"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return AccountInfo{}, fmt.Errorf("rpc getAccountInfo %s: %w", pubkey, err)
	}
	if out.Value == nil {
		return AccountInfo{Exists: false}, nil
	}
	acct := AccountInfo{Lamports: out.Value.Lamports, Owner: out.Value.Owner, Exists: true}
	if len(out.Value.Data) >= 2 {
		if s, ok := out.Value.Data[0].(string); ok {
			if enc, ok := out.Value.Data[1].(string); ok && enc == "base64" {
				acct.Data, _ = sol.DecodeB64(s)
			}
		}
	}
	return acct, nil
}

func (r *JSONRPC) GetTokenAccountsByOwner(ctx context.Context, owner, programID string) ([]TokenAccount, error) {
	raw, err := r.call(ctx, "getTokenAccountsByOwner", owner,
		map[string]interface{}{"programId": programID},
		map[string]interface{}{"encoding": "jsonParsed"})
	if err != nil {
		return nil, err
	}
	var out struct {
		Value []struct {
			Pubkey  string `json:"pubkey"`
			Account struct {
				Data struct {
					Parsed struct {
						Info struct {
							Mint  string `json:"mint"`
							Owner string `json:"owner"`
							// IsNative is false for tokens and an object for native
							// SOL; a raw field accepts both.
							IsNative    json.RawMessage `json:"isNative"`
							TokenAmount struct {
								Amount string `json:"amount"`
							} `json:"tokenAmount"`
						} `json:"info"`
					} `json:"parsed"`
				} `json:"data"`
			} `json:"account"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("rpc getTokenAccountsByOwner %s: %w", owner, err)
	}
	accounts := make([]TokenAccount, 0, len(out.Value))
	for _, v := range out.Value {
		info := v.Account.Data.Parsed.Info
		amount, err := parseU64(info.TokenAmount.Amount)
		if err != nil {
			continue
		}
		accounts = append(accounts, TokenAccount{
			Pubkey: v.Pubkey,
			Mint:   info.Mint,
			Amount: amount,
			Owner:  info.Owner,
		})
	}
	return accounts, nil
}

func (r *JSONRPC) RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error) {
	raw, err := r.call(ctx, "requestAirdrop", pubkey, lamports)
	if err != nil {
		return "", err
	}
	var out struct {
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("rpc requestAirdrop %s: %w", pubkey, err)
	}
	return out.Value, nil
}

func (r *JSONRPC) GetBalance(ctx context.Context, pubkey string) (uint64, error) {
	raw, err := r.call(ctx, "getBalance", pubkey)
	if err != nil {
		return 0, err
	}
	var out struct {
		Value uint64 `json:"value"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return 0, fmt.Errorf("rpc getBalance %s: %w", pubkey, err)
	}
	return out.Value, nil
}

func (r *JSONRPC) GetSignatureStatus(ctx context.Context, signature string) (SignatureStatus, error) {
	raw, err := r.call(ctx, "getSignatureStatuses", []string{signature})
	if err != nil {
		return SignatureStatus{}, err
	}
	var out struct {
		Value []*struct {
			Err           any     `json:"err"`
			Confirmations *uint64 `json:"confirmations"`
			Confirmation  *string `json:"confirmation"`
		} `json:"value"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return SignatureStatus{}, fmt.Errorf("rpc getSignatureStatuses: %w", err)
	}
	if len(out.Value) == 0 || out.Value[0] == nil {
		return SignatureStatus{Exists: false}, nil
	}
	v := out.Value[0]
	st := SignatureStatus{Exists: true}
	if v.Err != nil {
		st.Err = fmt.Sprintf("%v", v.Err)
	}
	if v.Confirmations != nil && *v.Confirmations > 0 {
		st.Confirmed = true
		st.Confirmations = v.Confirmations
	}
	if v.Confirmation != nil && *v.Confirmation == "finalized" {
		st.Confirmed = true
	}
	return st, nil
}

func parseU64(s string) (uint64, error) {
	var n uint64
	if s == "" {
		return 0, nil
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("parse u64: %q", s)
		}
		d := uint64(c - '0')
		if n > (^uint64(0)-d)/10 {
			return 0, fmt.Errorf("parse u64: overflow %q", s)
		}
		n = n*10 + d
	}
	return n, nil
}
