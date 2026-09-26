package identity

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/mrsirg97-rgb/rig/store"
)

type Role string

const (
	Architect Role = "architect"
	Worker    Role = "worker"
	Reviewer  Role = "reviewer"
)

func (r Role) Valid() bool {
	return r == Architect || r == Worker || r == Reviewer
}

func Roles() []Role { return []Role{Architect, Worker, Reviewer} }

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

type RoleDefaults struct {
	DefaultName string
	DefaultBio  string
	Cadence     string
	BlockSize   string
	Budget      float64
	Stall       int
	Timeout     int
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
	},
	Worker: {
		DefaultName: "torch worker",
		DefaultBio:  "Claims and completes tasks. Small stakes, steady fires.",
		Cadence:     "0 */2 * * *",
		BlockSize:   "compact",
		Budget:      0.5,
		Stall:       30,
		Timeout:     45,
	},
	Reviewer: {
		DefaultName: "torch reviewer",
		DefaultBio:  "Verdicts completed work: accept or reject, with a reason. A reject is a costly no.",
		Cadence:     "0 */6 * * *",
		BlockSize:   "full",
		Budget:      1,
		Stall:       60,
		Timeout:     60,
	},
}

func DefaultsFor(role Role) (RoleDefaults, error) {
	if d, ok := roleDefaults[role]; ok {
		return d, nil
	}
	if role == "" {
		return RoleDefaults{}, fmt.Errorf("identity: role required (architect, worker, reviewer)")
	}
	return RoleDefaults{}, fmt.Errorf("identity: unknown role %q (architect, worker, reviewer)", role)
}

const BaseStakeLamports uint64 = 10_000_000

func AgentID(wallet string, role Role) string {
	s := wallet
	if len(s) > 4 {
		s = s[len(s)-4:]
	}
	return "@AP" + strings.ToUpper(s) + "-" + string(role)
}

type Overrides struct {
	Name    string
	Bio     string
	Goal    string
	Cadence string
	Model   string
	Budget  float64
	Stall   int
	Timeout int
	Full    bool
}

type Row struct {
	ID        string
	Name      string
	Wallet    string
	Role      string
	Bio       string
	Goal      string
	Cadence   string
	Model     string
	Stall     int
	Budget    float64
	Timeout   int
	BlockSize string
	CreatedAt string
}

func NewRow(wallet string, role Role, o Overrides) (Row, error) {
	d, err := DefaultsFor(role)
	if err != nil {
		return Row{}, err
	}
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
		name = d.DefaultName
	}
	bio := o.Bio
	if bio == "" {
		bio = d.DefaultBio
	}
	return Row{
		ID:        AgentID(wallet, role),
		Name:      name,
		Wallet:    wallet,
		Role:      string(role),
		Bio:       bio,
		Goal:      strings.TrimSpace(o.Goal),
		Cadence:   cadence,
		Model:     o.Model,
		Stall:     stall,
		Budget:    budget,
		Timeout:   timeout,
		BlockSize: blockSize,
	}, nil
}

var Statements = []string{
	`CREATE TABLE IF NOT EXISTS identity (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		wallet TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'worker',
		bio TEXT NOT NULL,
		goal TEXT NOT NULL DEFAULT '',
		cadence TEXT NOT NULL,
		model TEXT NOT NULL,
		stall INTEGER NOT NULL DEFAULT 0,
		budget REAL NOT NULL DEFAULT 0,
		timeout INTEGER NOT NULL DEFAULT 0,
		block_size TEXT NOT NULL DEFAULT 'compact',
		created_at TEXT NOT NULL
	)`,
}

const SchemaVersion = 5

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
	if from < 5 {
		for _, stmt := range []string{
			`ALTER TABLE identity ADD COLUMN role TEXT NOT NULL DEFAULT 'worker'`,
			`ALTER TABLE identity ADD COLUMN goal TEXT NOT NULL DEFAULT ''`,
		} {
			if _, err := tx.Exec(stmt); err != nil {
				return "", err
			}
		}
		if _, err := tx.Exec(`UPDATE identity SET id = id || '-worker', role = 'worker'`); err != nil {
			return "", err
		}
	}
	return "", nil
}

const columns = `id, name, wallet, role, bio, goal, cadence, model, stall, budget, timeout, block_size, created_at`

func scan(row *sql.Row) (Row, error) {
	var r Row
	err := row.Scan(&r.ID, &r.Name, &r.Wallet, &r.Role, &r.Bio, &r.Goal, &r.Cadence, &r.Model, &r.Stall, &r.Budget, &r.Timeout, &r.BlockSize, &r.CreatedAt)
	if err == sql.ErrNoRows {
		return Row{}, fmt.Errorf("identity: no row (register first)")
	}
	return r, err
}

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

func GetByID(ctx context.Context, db store.DB, id string) (Row, error) {
	return scan(db.DB.QueryRowContext(ctx, `SELECT `+columns+` FROM identity WHERE id = ?`, id))
}

func List(ctx context.Context, db store.DB) ([]Row, error) {
	rows, err := db.DB.QueryContext(ctx, `SELECT `+columns+` FROM identity ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.ID, &r.Name, &r.Wallet, &r.Role, &r.Bio, &r.Goal, &r.Cadence, &r.Model, &r.Stall, &r.Budget, &r.Timeout, &r.BlockSize, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func Delete(ctx context.Context, db store.DB, id string) error {
	res, err := db.DB.ExecContext(ctx, `DELETE FROM identity WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("identity: delete %s: %w", id, err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("identity: no row %s", id)
	}
	return nil
}

func Upsert(ctx context.Context, db store.DB, r Row) error {
	if r.Name == "" || r.Wallet == "" || r.Model == "" {
		return fmt.Errorf("identity: name, wallet, and model are required")
	}
	role, err := ParseRole(r.Role)
	if err != nil {
		return err
	}
	r.Role = string(role)
	r.ID = AgentID(r.Wallet, role)
	if r.CreatedAt == "" {
		r.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	var existing string
	err = db.DB.QueryRowContext(ctx, `SELECT wallet FROM identity WHERE id = ?`, r.ID).Scan(&existing)
	switch {
	case err == nil && existing != r.Wallet:
		return fmt.Errorf("identity: wallet tag %s already belongs to %s", r.ID, existing)
	case err != nil && err != sql.ErrNoRows:
		return err
	}
	_, err = db.DB.ExecContext(ctx, `INSERT INTO identity (id, name, wallet, role, bio, goal, cadence, model, stall, budget, timeout, block_size, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name=excluded.name, wallet=excluded.wallet, role=excluded.role,
			bio=excluded.bio, goal=excluded.goal, cadence=excluded.cadence, model=excluded.model,
			stall=excluded.stall, budget=excluded.budget, timeout=excluded.timeout, block_size=excluded.block_size`,
		r.ID, r.Name, r.Wallet, r.Role, r.Bio, r.Goal, r.Cadence, r.Model, r.Stall, r.Budget, r.Timeout, r.BlockSize, r.CreatedAt)
	return err
}
