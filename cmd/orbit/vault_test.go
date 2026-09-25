package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/orbit/client"
	"github.com/mrsirg97-rgb/orbit/sol"
)

func TestVaultCreateSucceedsWithoutCreator(t *testing.T) {
	sends := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("rpc decode: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "getLatestBlockhash":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 1,
				"result": map[string]any{
					"context": map[string]any{"slot": 1},
					"value":   map[string]any{"blockhash": "11111111111111111111111111111111"},
				},
			})
		case "sendTransaction":
			sends++
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": 2, "result": "sigVaultCreate123",
			})
		default:
			t.Errorf("unexpected rpc method %q", req.Method)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config")
	cfg := "ORBIT_INDEXER=" + srv.URL + "\n" +
		"ORBIT_RPC=" + srv.URL + "\n" +
		"ORBIT_PROGRAM_ID=" + client.DevnetProgramID + "\n"
	if err := os.WriteFile(cfgFile, []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ORBIT_CONFIG", cfgFile)
	t.Setenv("ORBIT_INDEXER", srv.URL)
	t.Setenv("ORBIT_RPC", srv.URL)
	t.Setenv("ORBIT_PROGRAM_ID", client.DevnetProgramID)

	kp, err := sol.GenerateKeypair()
	if err != nil {
		t.Fatal(err)
	}
	if code := runVault([]string{"create", "--operator-key", sol.Encode(kp.Secret)}); code != 0 {
		t.Fatalf("vault create exit %d", code)
	}
	if sends != 1 {
		t.Errorf("sendTransaction calls: %d, want 1", sends)
	}
	b, err := os.ReadFile(cfgFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "ORBIT_VAULT_CREATOR="+kp.PublicBase58()) {
		t.Errorf("config missing the derived creator:\n%s", b)
	}
}
