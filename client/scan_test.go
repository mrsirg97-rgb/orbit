package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// serveRecordedTxs hosts the recorded transaction fixture as the JSON-RPC
// seam: getSignaturesForAddress + getTransaction, exactly as a node (or the
// indexer's /rpc proxy) answers.
func serveRecordedTxs(t *testing.T) *JSONRPC {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "testdata", "board_txs.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Signatures   []json.RawMessage          `json:"signatures"`
		Transactions map[string]json.RawMessage `json:"transactions"`
	}
	if err := json.Unmarshal(b, &fixture); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var req struct {
			Method string `json:"method"`
			Params []any  `json:"params"`
			ID     int64  `json:"id"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("content-type", "application/json")
		var result any
		switch req.Method {
		case "getSignaturesForAddress":
			result = fixture.Signatures
		case "getTransaction":
			sig, _ := req.Params[0].(string)
			if raw, ok := fixture.Transactions[sig]; ok {
				result = json.RawMessage(raw)
			} else {
				result = nil
			}
		default:
			http.Error(w, "unknown method", 400)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	t.Cleanup(srv.Close)
	return NewJSONRPC(srv.URL)
}

func TestScanMessagesAgainstRecordedTransactions(t *testing.T) {
	rpc := serveRecordedTxs(t)
	mint := "EtWTRABZaYq6iMfeYKouRu166VU2xqa1wcaWoxPkrZBG"
	rows, err := ScanMessages(context.Background(), rpc, DevnetProgramID, mint, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows: %d, want 2 (the failed tx is skipped)", len(rows))
	}
	first := rows[0]
	if first.MemoText != "[worker] claim 1" {
		t.Errorf("memo: %q", first.MemoText)
	}
	if first.Sender != "9BsnjSj5gNrkKKpCmWqSAPrDKtnH3yEwzJf5YzCX7MDz" {
		t.Errorf("sender: %s", first.Sender)
	}
	if first.ActionKind == nil || *first.ActionKind != "buy" {
		t.Errorf("action kind: %v", first.ActionKind)
	}
	if first.Slot != 110 {
		t.Errorf("slot: %d", first.Slot)
	}
	wantTime := time.Unix(1767268980, 0).UTC().Format(time.RFC3339)
	if first.CreatedAt != wantTime {
		t.Errorf("created_at: %s, want %s", first.CreatedAt, wantTime)
	}
	second := rows[1]
	if second.MemoText != "[architect] task 1: Context compaction" {
		t.Errorf("memo: %q", second.MemoText)
	}
	if second.ActionKind == nil || *second.ActionKind != "sell" {
		t.Errorf("action kind: %v", second.ActionKind)
	}
	if second.Slot != 115 {
		t.Errorf("slot: %d", second.Slot)
	}
	if rows[0].Slot > rows[1].Slot {
		t.Error("rows are not in chain order")
	}
}
