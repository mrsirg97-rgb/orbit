package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveRPC(t *testing.T, results map[string]json.RawMessage) *JSONRPC {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		var req struct {
			Method string `json:"method"`
			ID     int64  `json:"id"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		result, ok := results[req.Method]
		if !ok {
			http.Error(w, "unknown method", 400)
			return
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": json.RawMessage(result)})
	}))
	t.Cleanup(srv.Close)
	return NewJSONRPC(srv.URL)
}

func TestRequestAirdropDecodesBareSignature(t *testing.T) {
	// A real node returns the signature as a bare string, not {"value": ...}.
	rpc := serveRPC(t, map[string]json.RawMessage{
		"requestAirdrop": json.RawMessage(`"airdrop-sig-123"`),
	})
	sig, err := rpc.RequestAirdrop(context.Background(), "11111111111111111111111111111111", 1_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if sig != "airdrop-sig-123" {
		t.Errorf("signature %q, want the bare string", sig)
	}
}

func TestGetSignatureStatusDecodesConfirmationStatus(t *testing.T) {
	cases := []struct {
		name      string
		status    string
		exists    bool
		confirmed bool
	}{
		{"finalized with null confirmations", `{"confirmationStatus":"finalized","confirmations":null,"err":null}`, true, true},
		{"confirmed with a counter", `{"confirmationStatus":"confirmed","confirmations":2,"err":null}`, true, true},
		{"processed is not confirmed", `{"confirmationStatus":"processed","confirmations":null,"err":null}`, true, false},
		{"missing account", `null`, false, false},
	}
	for _, c := range cases {
		rpc := serveRPC(t, map[string]json.RawMessage{
			"getSignatureStatuses": json.RawMessage(`{"value":[` + c.status + `]}`),
		})
		st, err := rpc.GetSignatureStatus(context.Background(), "sig")
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if st.Exists != c.exists || st.Confirmed != c.confirmed {
			t.Errorf("%s: exists=%v confirmed=%v, want %v/%v", c.name, st.Exists, st.Confirmed, c.exists, c.confirmed)
		}
	}
}
