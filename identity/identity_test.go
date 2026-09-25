package identity

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/rig/store"
)

func TestDefaults(t *testing.T) {
	d := Defaults()
	if d.Cadence != "0 */2 * * *" || d.BlockSize != "compact" || d.Budget != 0.5 ||
		d.Stall != 30 || d.Timeout != 45 {
		t.Errorf("defaults %+v", d)
	}
	if d.Name == "" || d.Bio == "" {
		t.Errorf("defaults incomplete: %+v", d)
	}
}

func TestNewRowWithoutRole(t *testing.T) {
	const wallet = "So11111111111111111111111111111111111111112"
	row, err := NewRow(wallet, Overrides{Model: "dsv4"})
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != "@AP1112" {
		t.Errorf("id %s, want the wallet tag @AP1112", row.ID)
	}
	if row.Cadence != "0 */2 * * *" || row.BlockSize != "compact" || row.Budget != 0.5 ||
		row.Stall != 30 || row.Timeout != 45 {
		t.Errorf("defaults not applied: %+v", row)
	}
	if row.Name != "torch agent" || row.Model != "dsv4" {
		t.Errorf("name/model: %+v", row)
	}
	if row.Bio == "" || row.Wallet != wallet {
		t.Errorf("bio/wallet: %+v", row)
	}
}

func TestNewRowOverrides(t *testing.T) {
	const wallet = "So11111111111111111111111111111111111111112"
	row, err := NewRow(wallet, Overrides{
		Name: "Beta", Cadence: "0 1 * * *", Model: "qwen3.8-27b", Budget: 2.5,
		Stall: 45, Timeout: 90, Full: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if row.Name != "Beta" || row.Cadence != "0 1 * * *" || row.Model != "qwen3.8-27b" || row.Budget != 2.5 {
		t.Errorf("overrides lost: %+v", row)
	}
	if row.Stall != 45 || row.Timeout != 90 {
		t.Errorf("stall/timeout overrides lost: %+v", row)
	}
	if row.BlockSize != "full" {
		t.Errorf("block size %s, want the full override", row.BlockSize)
	}
}

func TestNewRowRefusesMissingModel(t *testing.T) {
	if _, err := NewRow("So11111111111111111111111111111111111111112", Overrides{}); err == nil || !strings.Contains(err.Error(), "model") {
		t.Errorf("missing model: %v", err)
	}
}

func TestUpsertOneRowPerWallet(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.sqlite")
	db, err := Store(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	ctx := context.Background()
	const wallet = "So11111111111111111111111111111111111111112"
	if err := Upsert(ctx, db, Row{Name: "Alpha", Wallet: wallet, Model: "dsv4"}); err != nil {
		t.Fatal(err)
	}
	// A second register for the same wallet is the same row, whatever id
	// the caller names — one row per wallet, never two.
	if err := Upsert(ctx, db, Row{Name: "Beta", Wallet: wallet, Model: "dsv4"}); err != nil {
		t.Fatal(err)
	}
	rows, err := List(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows: %d, want 1 per wallet", len(rows))
	}
	if rows[0].ID != "@AP1112" || rows[0].Name != "Beta" {
		t.Errorf("row: %+v", rows[0])
	}
}

func TestStoreMigrationFromV3DropsRoles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.sqlite")
	oldStatements := []string{
		`CREATE TABLE IF NOT EXISTS identity (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			wallet TEXT NOT NULL,
			bio TEXT NOT NULL,
			personality TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'worker',
			voice TEXT NOT NULL DEFAULT '',
			cadence TEXT NOT NULL,
			model TEXT NOT NULL,
			stall INTEGER NOT NULL DEFAULT 0,
			budget REAL NOT NULL DEFAULT 0,
			timeout INTEGER NOT NULL DEFAULT 0,
			block_size TEXT NOT NULL DEFAULT 'compact',
			stake_scale REAL NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL
		)`,
	}
	db, _, _, err := store.Open(path, oldStatements, 3)
	if err != nil {
		t.Fatal(err)
	}
	const wallet = "So11111111111111111111111111111111111111112"
	if _, err := db.DB.Exec(`INSERT INTO identity (id, name, wallet, bio, personality, role, voice, cadence, model, stall, budget, timeout, block_size, stake_scale, created_at)
		VALUES ('@AP1112-worker', 'worker agent', ?, 'bio', 'mercenary', 'worker', 'mercenary', '0 */2 * * *', 'dsv4', 30, 0.5, 45, 'compact', 0.25, '2026-08-15T12:00:00Z')`, wallet); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO identity (id, name, wallet, bio, personality, role, voice, cadence, model, stall, budget, timeout, block_size, stake_scale, created_at)
		VALUES ('@AP1112-architect', 'arch agent', ?, 'bio2', 'scout', 'architect', 'scout', '0 12 * * *', 'dsv4', 90, 5, 120, 'full', 4, '2026-08-16T12:00:00Z')`, wallet); err != nil {
		t.Fatal(err)
	}
	db.DB.Close()

	open, err := Store(path)
	if err != nil {
		t.Fatal(err)
	}
	defer open.DB.Close()
	rows, err := List(context.Background(), open)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("migrated rows: %d, want 1 per wallet", len(rows))
	}
	row := rows[0]
	if row.ID != "@AP1112" {
		t.Errorf("migrated id %s, want the wallet tag", row.ID)
	}
	if row.Name != "arch agent" {
		t.Errorf("migrated kept the wrong row: %+v", row)
	}
	if row.Cadence != "0 12 * * *" || row.BlockSize != "full" || row.Stall != 90 || row.Timeout != 120 || row.Budget != 5 {
		t.Errorf("migrated row lost fields: %+v", row)
	}
}

func TestStoreMigrationFromV2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.sqlite")
	oldStatements := []string{
		`CREATE TABLE IF NOT EXISTS identity (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			wallet TEXT NOT NULL,
			bio TEXT NOT NULL,
			personality TEXT NOT NULL,
			cadence TEXT NOT NULL,
			model TEXT NOT NULL,
			stall INTEGER NOT NULL DEFAULT 0,
			budget REAL NOT NULL DEFAULT 0,
			timeout INTEGER NOT NULL DEFAULT 0,
			block_size TEXT NOT NULL DEFAULT 'compact',
			created_at TEXT NOT NULL
		)`,
	}
	db, _, _, err := store.Open(path, oldStatements, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB.Exec(`INSERT INTO identity (id, name, wallet, bio, personality, cadence, model, stall, budget, timeout, block_size, created_at)
		VALUES ('@AP1112', 'torch agent', 'So11111111111111111111111111111111111111112', 'bio', 'mercenary', '0 */8 * * *', 'dsv4', 45, 1.25, 60, 'compact', '2026-08-15T12:00:00Z')`); err != nil {
		t.Fatal(err)
	}
	db.DB.Close()

	open, err := Store(path)
	if err != nil {
		t.Fatal(err)
	}
	defer open.DB.Close()
	row, err := GetByID(context.Background(), open, "@AP1112")
	if err != nil {
		t.Fatal(err)
	}
	if row.Cadence != "0 */8 * * *" || row.Model != "dsv4" || row.Budget != 1.25 {
		t.Errorf("migrated row lost fields: %+v", row)
	}
	if row.BlockSize != "compact" {
		t.Errorf("block size %s", row.BlockSize)
	}
}

func TestUpsertRefusesWalletTagCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "identity.sqlite")
	db, err := Store(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.DB.Close()
	ctx := context.Background()
	if err := Upsert(ctx, db, Row{Name: "A", Wallet: "So11111111111111111111111111111111111111112", Model: "dsv4"}); err != nil {
		t.Fatal(err)
	}
	// A different wallet whose last 4 chars collide with the first must
	// never silently re-own the row.
	other := "9" + strings.Repeat("9", 39) + "1112"
	if other == "So11111111111111111111111111111111111111112" {
		t.Fatal("test wallet construction collides")
	}
	if err := Upsert(ctx, db, Row{Name: "B", Wallet: other, Model: "dsv4"}); err == nil {
		t.Fatal("wallet tag collision accepted")
	}
}
