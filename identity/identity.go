// Package identity is the agent's local identity row: name, wallet, role,
// bio, personality, voice, cadence, model, and the job's stall/budget/
// timeout. One row per (wallet, role) — the role suffix is the agent id, so
// one wallet can host an architect, a worker, and a reviewer side by side.
// The on-chain AgentProfile checkpoint is a later PR.
package identity

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/rig/store"
)

// Role is rig's swarm role carried onto the chain. The role owns cadence,
// world size, budget, stall, timeout, and stake scale; it never changes with
// the voice.
type Role string

const (
	Architect Role = "architect"
	Worker    Role = "worker"
	Reviewer  Role = "reviewer"
)

func (r Role) Valid() bool {
	return r == Architect || r == Worker || r == Reviewer
}

// ParseRole accepts exactly one of the three roles; anything else refuses
// naming the three.
func ParseRole(s string) (Role, error) {
	r := Role(strings.TrimSpace(s))
	if r.Valid() {
		return r, nil
	}
	if s == "" {
		return "", fmt.Errorf("identity: role required (architect, worker, reviewer)")
	}
	return "", fmt.Errorf("identity: unknown role %q (architect, worker, reviewer)", s)
}

// RoleDefaults is one row of the role table: what a role may do (directive,
// memo shapes) and how its agent is configured.
type RoleDefaults struct {
	DefaultName string
	DefaultBio  string
	Cadence     string
	BlockSize   string
	Budget      float64
	Stall       int
	Timeout     int
	StakeScale  float64
	Directive   string
	MemoShapes  string
}

var roleDefaults = map[Role]RoleDefaults{
	Architect: {
		DefaultName: "torch architect",
		DefaultBio:  "Proposes and funds tasks on a project. A reject from a reviewer is a correction.",
		Cadence:     "0 12 * * *",
		BlockSize:   "full",
		Budget:      5,
		Stall:       90,
		Timeout:     120,
		StakeScale:  4,
		Directive:   "proposes and funds tasks on a project",
		MemoShapes:  "task | brief | accept",
	},
	Worker: {
		DefaultName: "torch worker",
		DefaultBio:  "Claims and completes tasks. Small stakes, steady fires.",
		Cadence:     "0 */2 * * *",
		BlockSize:   "compact",
		Budget:      0.5,
		Stall:       30,
		Timeout:     45,
		StakeScale:  0.25,
		Directive:   "claims and completes tasks",
		MemoShapes:  "claim | note | complete",
	},
	Reviewer: {
		DefaultName: "torch reviewer",
		DefaultBio:  "Verdicts completed work: accept or reject, with a reason. A reject is a costly no.",
		Cadence:     "0 */6 * * *",
		BlockSize:   "full",
		Budget:      1,
		Stall:       60,
		Timeout:     60,
		StakeScale:  1,
		Directive:   "verdicts completed work: accept or reject, with a reason",
		MemoShapes:  "accept | reject",
	},
}

// DefaultsFor returns the role's row; an unknown role refuses naming the
// three.
func DefaultsFor(role Role) (RoleDefaults, error) {
	if d, ok := roleDefaults[role]; ok {
		return d, nil
	}
	if role == "" {
		return RoleDefaults{}, fmt.Errorf("identity: role required (architect, worker, reviewer)")
	}
	return RoleDefaults{}, fmt.Errorf("identity: unknown role %q (architect, worker, reviewer)", role)
}

// Voices are Pyre's archetypes: they color the memo tone only and never
// change what the role may do.
var Voices = []string{"loyalist", "mercenary", "provocateur", "scout", "whale"}

// ValidVoice reports a Pyre archetype.
func ValidVoice(v string) bool {
	for _, a := range Voices {
		if a == v {
			return true
		}
	}
	return false
}

// BaseStakeLamports is the stake of one default action (0.01 SOL); the role's
// StakeScale scales it per action.
const BaseStakeLamports uint64 = 10_000_000

// StakeLamports applies the role's stake scale to the base stake.
func StakeLamports(scale float64) uint64 {
	return uint64(float64(BaseStakeLamports) * scale)
}

// AgentID is the identity row's key: the wallet's short tag plus the role.
func AgentID(wallet string, role Role) string {
	s := wallet
	if len(s) > 4 {
		s = s[len(s)-4:]
	}
	return "@AP" + strings.ToUpper(s) + "-" + string(role)
}

// TagMemo stamps the role onto a memo text so the board can fold who did
// what. The voice colors the text; the tag stays the role.
func TagMemo(role, text string) string {
	return "[" + role + "] " + strings.TrimSpace(text)
}

// Overrides are the register flags that beat the role defaults.
type Overrides struct {
	Name        string
	Bio         string
	Personality string
	Voice       string
	Cadence     string
	Model       string
	Budget      float64
	Stall       int
	Timeout     int
	Full        bool
}

// Row is one agent identity.
type Row struct {
	ID          string
	Name        string
	Wallet      string
	Bio         string
	Personality string
	Role        string
	Voice       string
	Cadence     string
	Model       string
	Stall       int
	Budget      float64
	Timeout     int
	BlockSize   string
	StakeScale  float64
	CreatedAt   string
}

// NewRow resolves the identity row from the role defaults and the overrides:
// cadence, world size, budget, stall, timeout, stake scale, name, and bio
// come from the role unless the flag overrides them. Model is always
// explicit — the operator picks the fleet model.
func NewRow(wallet string, role Role, o Overrides) (Row, error) {
	d, err := DefaultsFor(role)
	if err != nil {
		return Row{}, err
	}
	if strings.TrimSpace(o.Model) == "" {
		return Row{}, fmt.Errorf("identity: model required")
	}
	if o.Voice != "" && !ValidVoice(o.Voice) {
		return Row{}, fmt.Errorf("identity: unknown voice %q (%s)", o.Voice, strings.Join(Voices, ", "))
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
		name = d.DefaultName
	}
	bio := o.Bio
	if bio == "" {
		bio = d.DefaultBio
	}
	personality := o.Personality
	if personality == "" {
		personality = "mercenary"
	}
	voice := o.Voice
	if voice == "" {
		voice = personality
	}
	return Row{
		ID:          AgentID(wallet, role),
		Name:        name,
		Wallet:      wallet,
		Bio:         bio,
		Personality: personality,
		Role:        string(role),
		Voice:       voice,
		Cadence:     cadence,
		Model:       o.Model,
		Stall:       stall,
		Budget:      budget,
		Timeout:     timeout,
		BlockSize:   blockSize,
		StakeScale:  d.StakeScale,
	}, nil
}

// Statements is the identity schema.
var Statements = []string{
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

const SchemaVersion = 3

// migration adds block_size (v2), then role/voice/stake_scale (v3) to stores
// created before the roles landed.
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
	return "", nil
}

const columns = `id, name, wallet, bio, personality, role, voice, cadence, model, stall, budget, timeout, block_size, stake_scale, created_at`

func scan(row *sql.Row) (Row, error) {
	var r Row
	err := row.Scan(&r.ID, &r.Name, &r.Wallet, &r.Bio, &r.Personality, &r.Role, &r.Voice, &r.Cadence, &r.Model, &r.Stall, &r.Budget, &r.Timeout, &r.BlockSize, &r.StakeScale, &r.CreatedAt)
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
		if err := rows.Scan(&r.ID, &r.Name, &r.Wallet, &r.Bio, &r.Personality, &r.Role, &r.Voice, &r.Cadence, &r.Model, &r.Stall, &r.Budget, &r.Timeout, &r.BlockSize, &r.StakeScale, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Upsert writes the identity row. The id is the (wallet, role) key; a
// different wallet for the same id is an error (never silently re-own).
func Upsert(ctx context.Context, db store.DB, r Row) error {
	if r.ID == "" || r.Name == "" || r.Wallet == "" || r.Model == "" {
		return fmt.Errorf("identity: id, name, wallet, and model are required")
	}
	if _, err := DefaultsFor(Role(r.Role)); err != nil {
		return err
	}
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := db.DB.ExecContext(ctx, `INSERT INTO identity (id, name, wallet, bio, personality, role, voice, cadence, model, stall, budget, timeout, block_size, stake_scale, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, wallet=excluded.wallet, bio=excluded.bio,
			personality=excluded.personality, role=excluded.role, voice=excluded.voice,
			cadence=excluded.cadence, model=excluded.model, stall=excluded.stall,
			budget=excluded.budget, timeout=excluded.timeout, block_size=excluded.block_size,
			stake_scale=excluded.stake_scale`,
		r.ID, r.Name, r.Wallet, r.Bio, r.Personality, r.Role, r.Voice, r.Cadence, r.Model, r.Stall, r.Budget, r.Timeout, r.BlockSize, r.StakeScale, r.CreatedAt)
	return err
}
