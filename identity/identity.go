// Package identity is the agent's local identity row: name, wallet, bio,
// cadence, model, and the job's stall/budget/timeout. One row per wallet —
// the wallet is the agent's identity, never a role label. The on-chain
// AgentProfile checkpoint is a later PR.
package identity

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/rig/store"
)

// RegisterDefaults is the one set of register defaults: cadence, world
// size, budget, stall, and timeout. No roles — every wallet registers with
// the same sensible defaults unless the flags override them.
type RegisterDefaults struct {
	Name      string
	Bio       string
	Cadence   string
	BlockSize string
	Budget    float64
	Stall     int
	Timeout   int
}

var defaults = RegisterDefaults{
	Name:      "torch agent",
	Bio:       "A torch contributor. Proposes and funds tasks, claims and completes work, verdicts with a reason.",
	Cadence:   "0 */2 * * *",
	BlockSize: "compact",
	Budget:    0.5,
	Stall:     30,
	Timeout:   45,
}

// Defaults returns the one set of register defaults.
func Defaults() RegisterDefaults { return defaults }

// BaseStakeLamports is the default per-action stake (0.01 SOL) for market
// writes; the board's memo buy is separate (client.MemoBuyLamports).
const BaseStakeLamports uint64 = 10_000_000

// AgentID is the identity row's key: the wallet's short tag. One row per
// wallet — no role suffix.
func AgentID(wallet string) string {
	s := wallet
	if len(s) > 4 {
		s = s[len(s)-4:]
	}
	return "@AP" + strings.ToUpper(s)
}

// Overrides are the register flags that beat the defaults.
type Overrides struct {
	Name    string
	Bio     string
	Cadence string
	Model   string
	Budget  float64
	Stall   int
	Timeout int
	Full    bool
}

// Row is one agent identity: one row per wallet.
type Row struct {
	ID        string
	Name      string
	Wallet    string
	Bio       string
	Cadence   string
	Model     string
	Stall     int
	Budget    float64
	Timeout   int
	BlockSize string
	CreatedAt string
}

// NewRow resolves the identity row from the defaults and the overrides:
// cadence, world size, budget, stall, and timeout come from the defaults
// unless the flag overrides them. Model is always explicit — the operator
// picks the fleet model.
func NewRow(wallet string, o Overrides) (Row, error) {
	d := defaults
	if strings.TrimSpace(o.Model) == "" {
		return Row{}, fmt.Errorf("identity: model required")
	}
	blockSize := d.BlockSize
	if o.Full {
		blockSize = "full"
	}
	cadence := o.Cadence
	if cadence == "" {
		cadence = d.Cadence
	}
	budget := o.Budget
	if budget == 0 {
		budget = d.Budget
	}
	stall := o.Stall
	if stall == 0 {
		stall = d.Stall
	}
	timeout := o.Timeout
	if timeout == 0 {
		timeout = d.Timeout
	}
	name := o.Name
	if name == "" {
		name = d.Name
	}
	bio := o.Bio
	if bio == "" {
		bio = d.Bio
	}
	return Row{
		ID:        AgentID(wallet),
		Name:      name,
		Wallet:    wallet,
		Bio:       bio,
		Cadence:   cadence,
		Model:     o.Model,
		Stall:     stall,
		Budget:    budget,
		Timeout:   timeout,
		BlockSize: blockSize,
	}, nil
}

// Statements is the identity schema.
var Statements = []string{
	`CREATE TABLE IF NOT EXISTS identity (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		wallet TEXT NOT NULL,
		bio TEXT NOT NULL,
		cadence TEXT NOT NULL,
		model TEXT NOT NULL,
		stall INTEGER NOT NULL DEFAULT 0,
		budget REAL NOT NULL DEFAULT 0,
		timeout INTEGER NOT NULL DEFAULT 0,
		block_size TEXT NOT NULL DEFAULT 'compact',
		created_at TEXT NOT NULL
	)`,
}

const SchemaVersion = 4

// migration adds block_size (v2), then role/voice/stake_scale (v3) to
// stores created before the roles landed; v4 removes the role table: one
// row per wallet (the newest survives), the id becomes the wallet tag, and
// the role/voice/personality/stake_scale columns are dropped.
func migration(tx *sql.Tx, from, to int) (string, error) {
	if from < 2 {
		if _, err := tx.Exec(`ALTER TABLE identity ADD COLUMN block_size TEXT NOT NULL DEFAULT 'compact'`); err != nil {
			return "", err
		}
	}
	if from < 3 {
		for _, stmt := range []string{
			`ALTER TABLE identity ADD COLUMN role TEXT NOT NULL DEFAULT 'worker'`,
			`ALTER TABLE identity ADD COLUMN voice TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE identity ADD COLUMN stake_scale REAL NOT NULL DEFAULT 1`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return "", err
			}
		}
	}
	if from < 4 {
		if _, err := tx.Exec(`DELETE FROM identity WHERE rowid NOT IN (SELECT MAX(rowid) FROM identity GROUP BY wallet)`); err != nil {
			return "", err
		}
		if _, err := tx.Exec(`UPDATE identity SET id = '@AP' || upper(substr(wallet, -4))`); err != nil {
			return "", err
		}
		for _, stmt := range []string{
			`ALTER TABLE identity DROP COLUMN role`,
			`ALTER TABLE identity DROP COLUMN voice`,
			`ALTER TABLE identity DROP COLUMN personality`,
			`ALTER TABLE identity DROP COLUMN stake_scale`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return "", err
			}
		}
	}
	return "", nil
}

const columns = `id, name, wallet, bio, cadence, model, stall, budget, timeout, block_size, created_at`

func scan(row *sql.Row) (Row, error) {
	var r Row
	err := row.Scan(&r.ID, &r.Name, &r.Wallet, &r.Bio, &r.Cadence, &r.Model, &r.Stall, &r.Budget, &r.Timeout, &r.BlockSize, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return Row{}, fmt.Errorf("identity: no row (register first)")
	}
	return r, err
}

// Store opens the identity store at path.
func Store(path string) (store.DB, error) {
	db, quarantined, report, err := store.Open(path, Statements, SchemaVersion, migration)
	if err != nil {
		return store.DB{}, err
	}
	if quarantined != "" {
		return store.DB{}, fmt.Errorf("identity: quarantined %s", quarantined)
	}
	if report != "" {
		return store.DB{}, fmt.Errorf("identity: %s", report)
	}
	return db, nil
}

// Get returns the newest identity row (the single-agent path: direct -p
// worker use, and the legacy one-row store).
func Get(ctx context.Context, db store.DB) (Row, error) {
	return scan(db.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM identity ORDER BY rowid DESC LIMIT 1`))
}

// GetByID returns one identity row.
func GetByID(ctx context.Context, db store.DB, id string) (Row, error) {
	return scan(db.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM identity WHERE id = ?`, id))
}

// List returns every identity row.
func List(ctx context.Context, db store.DB) ([]Row, error) {
	rows, err := db.DB.QueryContext(ctx, `SELECT `+columns+` FROM identity ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Name, &r.Wallet, &r.Bio, &r.Cadence, &r.Model, &r.Stall, &r.Budget, &r.Timeout, &r.BlockSize, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Upsert writes the identity row. The id is the wallet tag (one row per
// wallet, computed from the wallet — never a caller-supplied label), so a
// wallet cannot register twice under different ids.
func Upsert(ctx context.Context, db store.DB, r Row) error {
	if r.Name == "" || r.Wallet == "" || r.Model == "" {
		return fmt.Errorf("identity: name, wallet, and model are required")
	}
	r.ID = AgentID(r.Wallet)
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	var existing string
	err := db.DB.QueryRowContext(ctx, `SELECT wallet FROM identity WHERE id = ?`, r.ID).Scan(&existing)
	switch {
	case err == nil && existing != r.Wallet:
		return fmt.Errorf("identity: wallet tag %s already belongs to %s", r.ID, existing)
	case err != nil && err != sql.ErrNoRows:
		return err
	}
	_, err = db.DB.ExecContext(ctx, `INSERT INTO identity (id, name, wallet, bio, cadence, model, stall, budget, timeout, block_size, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, wallet=excluded.wallet, bio=excluded.bio,
			cadence=excluded.cadence, model=excluded.model, stall=excluded.stall,
			budget=excluded.budget, timeout=excluded.timeout, block_size=excluded.block_size`,
		r.ID, r.Name, r.Wallet, r.Bio, r.Cadence, r.Model, r.Stall, r.Budget, r.Timeout, r.BlockSize, r.CreatedAt)
	return err
}
