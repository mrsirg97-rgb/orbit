// Package identity is the agent's local identity row: name, wallet, bio,
// personality, cadence, model, and the job's stall/budget/timeout. One row
// per agent (the on-chain AgentProfile checkpoint is a later PR).
package identity

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mrsirg97-rgb/rig/store"
)

// Row is one agent identity.
type Row struct {
	ID          string // @APxxxx
	Name        string
	Wallet      string // agent hot wallet pubkey
	Bio         string
	Personality string
	Cadence     string // 5-field cron
	Model       string
	Stall       int
	Budget      float64
	Timeout     int
	CreatedAt   string
}

// DefaultCadence maps a personality to the Pyre PERSONALITY_INTERVALS cadence.
func DefaultCadence(personality string) string {
	switch personality {
	case "loyalist":
		return "0 */6 * * *"
	case "mercenary":
		return "0 */8 * * *"
	case "provocateur":
		return "0 */4 * * *"
	case "scout":
		return "0 */12 * * *"
	case "whale":
		return "0 12 * * *"
	default:
		return "0 */8 * * *"
	}
}

// Statements is the identity schema.
var Statements = []string{
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
		created_at TEXT NOT NULL
	)`,
}

const SchemaVersion = 1

// Store opens the identity store at path.
func Store(path string) (store.DB, error) {
	db, quarantined, report, err := store.Open(path, Statements, SchemaVersion)
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

// Get returns the identity row (the single agent).
func Get(ctx context.Context, db store.DB) (Row, error) {
	row := db.DB.QueryRowContext(ctx, `SELECT id, name, wallet, bio, personality, cadence, model, stall, budget, timeout, created_at FROM identity LIMIT 1`)
	var r Row
	err := row.Scan(&r.ID, &r.Name, &r.Wallet, &r.Bio, &r.Personality, &r.Cadence, &r.Model, &r.Stall, &r.Budget, &r.Timeout, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return Row{}, fmt.Errorf("identity: no row (register first)")
	}
	return r, err
}

// Upsert writes the identity row. The id is the identity's own key; a
// different wallet for the same id is an error (never silently re-own).
func Upsert(ctx context.Context, db store.DB, r Row) error {
	if r.ID == "" || r.Name == "" || r.Wallet == "" || r.Model == "" {
		return fmt.Errorf("identity: id, name, wallet, and model are required")
	}
	if r.Cadence == "" {
		r.Cadence = DefaultCadence(r.Personality)
	}
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := db.DB.ExecContext(ctx, `INSERT INTO identity (id, name, wallet, bio, personality, cadence, model, stall, budget, timeout, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, wallet=excluded.wallet, bio=excluded.bio,
			personality=excluded.personality, cadence=excluded.cadence, model=excluded.model,
			stall=excluded.stall, budget=excluded.budget, timeout=excluded.timeout`,
		r.ID, r.Name, r.Wallet, r.Bio, r.Personality, r.Cadence, r.Model, r.Stall, r.Budget, r.Timeout, r.CreatedAt)
	return err
}
