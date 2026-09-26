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

const RentExemptZeroData uint64 = 890880

type AccountInfo struct {
	Lamports uint64
	Owner    string
	Data     []byte
	Exists   bool
}

type TokenAccount struct {
	Pubkey   string
	Mint     string
	Amount   uint64
	Owner    string
	IsNative bool
}

type SignatureStatus struct {
	Exists    bool
	Confirmed bool
	Err       string
}

type SignatureInfo struct {
	Signature string
	BlockTime *int64
	Err       string
}

type TxInstruction struct {
	ProgramID string
	Data      []byte
}

type Transaction struct {
	Slot      int64
	BlockTime *int64
	Keys      []string
	Ixs       []TxInstruction
	InnerIxs  []TxInstruction
	Err       bool
}

type RPC interface {
	GetLatestBlockhash(ctx context.Context) (string, error)
	SendTransaction(ctx context.Context, signedRaw []byte) (string, error)
	GetAccountInfo(ctx context.Context, pubkey string) (AccountInfo, error)
	GetTokenAccountsByOwner(ctx context.Context, owner, programID string) ([]TokenAccount, error)
	GetBalance(ctx context.Context, pubkey string) (uint64, error)
	GetSignatureStatus(ctx context.Context, signature string) (SignatureStatus, error)

	RequestAirdrop(ctx context.Context, pubkey string, lamports uint64) (string, error)

	GetSignaturesForAddress(ctx context.Context, address string, limit int) ([]SignatureInfo, error)

	GetTransaction(ctx context.Context, signature string) (*Transaction, error)
}

type JSONRPC struct {
	base string
	cli  *http.Client
	id   atomic.Int64
}

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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.base, bytes.NewReader(body))
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
	var sig string
	if err := json.Unmarshal(raw, &sig); err != nil {
		return "", fmt.Errorf("rpc requestAirdrop %s: %w", pubkey, err)
	}
	return sig, nil
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
			Err any `json:"err"`
			// confirmations is null once the tx is finalized; the
			// confirmationStatus field is the real signal.
			ConfirmationStatus *string `json:"confirmationStatus"`
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
	if v.ConfirmationStatus != nil && (*v.ConfirmationStatus == "confirmed" || *v.ConfirmationStatus == "finalized") {
		st.Confirmed = true
	}
	return st, nil
}

func (r *JSONRPC) GetSignaturesForAddress(ctx context.Context, address string, limit int) ([]SignatureInfo, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}
	raw, err := r.call(ctx, "getSignaturesForAddress", address,
		map[string]interface{}{"limit": limit, "commitment": "confirmed"})
	if err != nil {
		return nil, err
	}
	var out []struct {
		Signature string          `json:"signature"`
		Err       json.RawMessage `json:"err"`
		BlockTime *int64          `json:"blockTime"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("rpc getSignaturesForAddress %s: %w", address, err)
	}
	infos := make([]SignatureInfo, 0, len(out))
	for _, s := range out {
		info := SignatureInfo{Signature: s.Signature, BlockTime: s.BlockTime}
		if s.Err != nil && string(s.Err) != "null" {
			info.Err = string(s.Err)
		}
		infos = append(infos, info)
	}
	return infos, nil
}

func (r *JSONRPC) GetTransaction(ctx context.Context, signature string) (*Transaction, error) {
	raw, err := r.call(ctx, "getTransaction", signature,
		map[string]interface{}{
			"encoding":                       "json",
			"maxSupportedTransactionVersion": 0,
			"commitment":                     "confirmed",
		})
	if err != nil {
		return nil, err
	}
	if string(raw) == "null" {
		return nil, fmt.Errorf("rpc getTransaction %s: not found", signature)
	}
	var out struct {
		Slot      int64  `json:"slot"`
		BlockTime *int64 `json:"blockTime"`
		Meta      struct {
			Err               json.RawMessage `json:"err"`
			InnerInstructions []struct {
				Instructions []jsonIx `json:"instructions"`
			} `json:"innerInstructions"`
		} `json:"meta"`
		Transaction struct {
			Message struct {
				AccountKeys  []string `json:"accountKeys"`
				Instructions []jsonIx `json:"instructions"`
			} `json:"message"`
		} `json:"transaction"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("rpc getTransaction %s: decode: %w", signature, err)
	}
	tx := &Transaction{
		Slot: out.Slot, BlockTime: out.BlockTime, Keys: out.Transaction.Message.AccountKeys,
		Err: out.Meta.Err != nil && string(out.Meta.Err) != "null",
	}
	decode := func(i jsonIx) (TxInstruction, error) {
		if i.ProgramIDIndex < 0 || i.ProgramIDIndex >= len(tx.Keys) {
			return TxInstruction{}, fmt.Errorf("rpc getTransaction %s: program id index %d out of range", signature, i.ProgramIDIndex)
		}
		data, err := sol.Decode(i.Data)
		if err != nil {
			return TxInstruction{}, fmt.Errorf("rpc getTransaction %s: instruction data: %w", signature, err)
		}
		return TxInstruction{ProgramID: tx.Keys[i.ProgramIDIndex], Data: data}, nil
	}
	for _, ix := range out.Transaction.Message.Instructions {
		d, err := decode(ix)
		if err != nil {
			return nil, err
		}
		tx.Ixs = append(tx.Ixs, d)
	}
	for _, inner := range out.Meta.InnerInstructions {
		for _, ix := range inner.Instructions {
			d, err := decode(ix)
			if err != nil {
				return nil, err
			}
			tx.InnerIxs = append(tx.InnerIxs, d)
		}
	}
	return tx, nil
}

type jsonIx struct {
	ProgramIDIndex int    `json:"programIdIndex"`
	Data           string `json:"data"`
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
