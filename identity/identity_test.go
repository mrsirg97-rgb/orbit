package identity

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mrsirg97-rgb/rig/store"
)

func TestRoleDefaults(t *testing.T) {
	cases := []struct {
		role       Role
		cadence    string
		block      string
		budget     float64
		stall      int
		timeout    int
		stakeScale float64
		shapes     string
	}{
		{Architect, "0 12 * * *", "full", 5, 90, 120, 4, "task | brief | accept"},
		{Worker, "0 */2 * * *", "compact", 0.5, 30, 45, 0.25, "claim | note | complete"},
		{Reviewer, "0 */6 * * *", "full", 1, 60, 60, 1, "accept | reject"},
	}
	for _, c := range cases {
		d, err := DefaultsFor(c.role)
		if err != nil {
			t.Fatalf("%s: %v", c.role, err)
		}
		if d.Cadence != c.cadence || d.BlockSize != c.block || d.Budget != c.budget ||
			d.Stall != c.stall || d.Timeout != c.timeout || d.StakeScale != c.stakeScale {
			t.Errorf("%s defaults %+v", c.role, d)
		}
		if !strings.Contains(d.MemoShapes, c.shapes) {
			t.Errorf("%s memo shapes %q, want %q", c.role, d.MemoShapes, c.shapes)
		}
		if d.Directive == "" || d.DefaultName == "" || d.DefaultBio == "" {
			t.Errorf("%s defaults incomplete: %+v", c.role, d)
		}
	}
}

func TestUnknownRoleRefusesNamingTheThree(t *testing.T) {
	for _, in := range []string{"pirate", "", "ARCHITECT", "architect,worker"} {
		_, err := ParseRole(in)
		if err == nil {
			t.Fatalf("ParseRole(%q) accepted", in)
		}
		for _, name := range []string{"architect", "worker", "reviewer"} {
			if !strings.Contains(err.Error(), name) {
				t.Errorf("ParseRole(%q) error %q does not name %q", in, err, name)
			}
		}
	}
	if got, err := ParseRole(" worker "); err != nil || got != Worker {
		t.Errorf("trimmed role: %v, %v", got, err)
	}
}

func TestNewRowAppliesDefaultsAndOverrides(t *testing.T) {
	const wallet = "So11111111111111111111111111111111111111112"
	row, err := NewRow(wallet, Worker, Overrides{Model: "dsv4"})
	if err != nil {
		t.Fatal(err)
	}
	if row.ID != "@AP1112-worker" {
		t.Errorf("id %s", row.ID)
	}
	if row.Cadence != "0 */2 * * *" || row.BlockSize != "compact" || row.Budget != 0.5 ||
		row.Stall != 30 || row.Timeout != 45 || row.StakeScale != 0.25 {
		t.Errorf("role defaults not applied: %+v", row)
	}
	if row.Voice != "mercenary" {
		t.Errorf("voice %q, want the default personality", row.Voice)
	}
	if row.Name != "torch worker" {
		t.Errorf("name %q", row.Name)
	}

	over, err := NewRow(wallet, Worker, Overrides{
		Name: "Beta", Cadence: "0 1 * * *", Model: "qwen3.8-27b", Budget: 2.5,
		Voice: "scout",
	})
	if err != nil {
		t.Fatal(err)
	}
	if over.Name != "Beta" || over.Cadence != "0 1 * * *" || over.Model != "qwen3.8-27b" || over.Budget != 2.5 {
		t.Errorf("overrides lost: %+v", over)
	}
	if over.Voice != "scout" {
		t.Errorf("voice override lost: %q", over.Voice)
	}
	if over.Stall != 30 || over.Timeout != 45 || over.BlockSize != "compact" {
		t.Errorf("role defaults clobbered by overrides: %+v", over)
	}
}

func TestNewRowArchitectAndReviewer(t *testing.T) {
	const wallet = "So11111111111111111111111111111111111111112"
	for _, c := range []struct {
		role Role
		id   string
	}{
		{Architect, "@AP1112-architect"},
		{Reviewer, "@AP1112-reviewer"},
	} {
		row, err := NewRow(wallet, c.role, Overrides{Model: "dsv4"})
		if err != nil {
			t.Fatal(err)
		}
		if row.ID != c.id {
			t.Errorf("%s id %s, want %s", c.role, row.ID, c.id)
		}
	}
}

func TestNewRowRefusesMissingModel(t *testing.T) {
	if _, err := NewRow("So11111111111111111111111111111111111111112", Worker, Overrides{}); err == nil || !strings.Contains(err.Error(), "model") {
		t.Errorf("missing model: %v", err)
	}
}

func TestNewRowRefusesUnknownVoice(t *testing.T) {
	_, err := NewRow("So11111111111111111111111111111111111111112", Worker, Overrides{Model: "dsv4", Voice: "pirate"})
	if err == nil {
		t.Fatal("unknown voice accepted")
	}
	for _, v := range Voices {
		if !strings.Contains(err.Error(), v) {
			t.Errorf("voice error does not name %s: %v", v, err)
		}
	}
}

func TestTagMemoCarriesRole(t *testing.T) {
	memo := TagMemo("worker", "claim: t3 — backed it")
	if !strings.HasPrefix(memo, "[worker] ") {
		t.Errorf("memo %q lacks the role tag", memo)
	}
	if !strings.Contains(memo, "claim: t3") {
		t.Errorf("memo lost the text: %q", memo)
	}
}

func TestStakeLamports(t *testing.T) {
	if got := StakeLamports(0.25); got != 2_500_000 {
		t.Errorf("0.25x stake %d, want 2500000", got)
	}
	if got := StakeLamports(4); got != 40_000_000 {
		t.Errorf("4x stake %d, want 40000000", got)
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
	if row.Role != "worker" {
		t.Errorf("migrated role %q, want worker", row.Role)
	}
	if row.StakeScale != 1 {
		t.Errorf("migrated stake scale %v, want 1", row.StakeScale)
	}
	if row.BlockSize != "compact" || row.Model != "dsv4" {
		t.Errorf("migrated row lost fields: %+v", row)
	}
}
