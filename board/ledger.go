package board

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mrsirg97-rgb/rig/v2/store/sqlx"

	"github.com/mrsirg97-rgb/orbit/client"
)

const (
	BackingContract = "contract"
	BackingWork     = "work"
)

type Ledger struct {
	Public   bool
	Carriers map[string]client.Carrier
	Ends     map[client.PositionKey]string
}

func (l Ledger) with(signature string, c client.Carrier) Ledger {
	carriers := make(map[string]client.Carrier, len(l.Carriers)+1)
	for k, v := range l.Carriers {
		carriers[k] = v
	}
	carriers[signature] = c
	l.Carriers = carriers
	return l
}

func backingOf(c client.Carrier) string {
	switch {
	case c.Long:
		return BackingWork
	case c.Lamports > client.MemoBuyLamports:
		return BackingContract
	}
	return ""
}

func IsPublic(status client.MarketStatus) bool { return status == client.StatusMigrated }

type ledgerUpdate struct {
	status      client.MarketStatus
	statusKnown bool
	carriers    map[string]client.Carrier
	ends        map[client.PositionKey]string
	endsKnown   bool
}

func (s *Store) ledgerUpdate(ctx context.Context, p Project, tc *client.TorchClient, rows []client.MessageRow) (ledgerUpdate, error) {
	up := ledgerUpdate{carriers: map[string]client.Carrier{}}
	if market, err := s.Market(ctx, p.Mint); err == nil {
		up.status, up.statusKnown = market.Status, true
	}
	if ends, err := tc.LongEnds(ctx, p.Mint); err == nil && ends != nil {
		up.ends, up.endsKnown = ends, true
	}
	missing, err := s.carriersMissing(ctx, p.Mint, rows)
	if err != nil {
		return up, err
	}
	for _, sig := range missing {
		c, err := tc.Carrier(ctx, sig)
		if err != nil {
			return up, fmt.Errorf("carrier %s: %w", sig, err)
		}
		up.carriers[sig] = c
	}
	return up, nil
}

func carried(verb string) bool { return verb == "claim" || verb == "reject" }

func (s *Store) carriersMissing(ctx context.Context, mint string, rows []client.MessageRow) ([]string, error) {
	bound, tx, err := s.DB.TxReadOnly(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var out []string
	for _, r := range rows {
		m, ok := ParseMemo(r.MemoText)
		if !ok || !carried(m.Verb) {
			continue
		}
		var one int
		err := tx.QueryRowContext(bound, `SELECT 1 FROM message_carriers WHERE project = ? AND signature = ?`, mint, r.Signature).Scan(&one)
		if err == sql.ErrNoRows {
			out = append(out, r.Signature)
			continue
		}
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func cacheLedger(ctx context.Context, txr *sql.Tx, mint string, up ledgerUpdate) error {
	if up.statusKnown {
		if _, err := txr.ExecContext(ctx,
			`INSERT INTO project_states (project, status) VALUES (?, ?)
			 ON CONFLICT(project) DO UPDATE SET status = excluded.status`, mint, string(up.status)); err != nil {
			return fmt.Errorf("state: %w", err)
		}
	}
	for sig, c := range up.carriers {
		opened := ""
		if c.Long {
			opened = string(client.SideLong)
		} else if c.Short {
			opened = string(client.SideShort)
		}
		if _, err := txr.ExecContext(ctx,
			`INSERT INTO message_carriers (project, signature, lamports, opened, vault, position_index, collateral)
			 VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT(project, signature) DO NOTHING`,
			mint, sig, int64(c.Lamports), opened, c.Vault, int64(c.Index), int64(c.Collateral)); err != nil {
			return fmt.Errorf("carrier: %w", err)
		}
	}
	if up.endsKnown {
		if _, err := txr.ExecContext(ctx, `DELETE FROM project_positions WHERE project = ?`, mint); err != nil {
			return fmt.Errorf("positions: %w", err)
		}
		for key, at := range up.ends {
			if _, err := txr.ExecContext(ctx,
				`INSERT INTO project_positions (project, vault, position_index, ended_at) VALUES (?, ?, ?, ?)`,
				mint, key.Vault, int64(key.Index), at); err != nil {
				return fmt.Errorf("positions: %w", err)
			}
		}
	}
	return nil
}

func ledgerIn(bound context.Context, mint string) (Ledger, error) {
	tx, err := sqlx.TxFrom(bound)
	if err != nil {
		return Ledger{}, err
	}
	l := Ledger{Carriers: map[string]client.Carrier{}, Ends: map[client.PositionKey]string{}}
	var status string
	err = tx.QueryRowContext(bound, `SELECT status FROM project_states WHERE project = ?`, mint).Scan(&status)
	if err != nil && err != sql.ErrNoRows {
		return Ledger{}, err
	}
	l.Public = IsPublic(client.MarketStatus(status))
	rows, err := tx.QueryContext(bound, `SELECT signature, lamports, opened, vault, position_index, collateral FROM message_carriers WHERE project = ?`, mint)
	if err != nil {
		return Ledger{}, err
	}
	for rows.Next() {
		var sig, opened, vault string
		var lamports, index, collateral int64
		if err := rows.Scan(&sig, &lamports, &opened, &vault, &index, &collateral); err != nil {
			rows.Close()
			return Ledger{}, err
		}
		l.Carriers[sig] = client.Carrier{
			Lamports: uint64(lamports), Long: opened == string(client.SideLong), Short: opened == string(client.SideShort),
			Vault: vault, Index: uint32(index), Collateral: uint64(collateral),
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Ledger{}, err
	}
	rows.Close()
	ends, err := tx.QueryContext(bound, `SELECT vault, position_index, ended_at FROM project_positions WHERE project = ?`, mint)
	if err != nil {
		return Ledger{}, err
	}
	defer ends.Close()
	for ends.Next() {
		var vault, at string
		var index int64
		if err := ends.Scan(&vault, &index, &at); err != nil {
			return Ledger{}, err
		}
		l.Ends[client.PositionKey{Vault: vault, Index: uint32(index)}] = at
	}
	return l, ends.Err()
}

type Backing struct {
	Backing     string
	Stake       uint64
	Position    client.PositionKey
	RejectShort uint64
}

func backingsIn(bound context.Context, mint string) (map[string]Backing, error) {
	tx, err := sqlx.TxFrom(bound)
	if err != nil {
		return nil, err
	}
	rows, err := tx.QueryContext(bound, `SELECT id, backing, stake, vault, position_index, reject_short FROM task_backing WHERE project = ?`, mint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]Backing{}
	for rows.Next() {
		var id, backing, vault string
		var stake, index, short int64
		if err := rows.Scan(&id, &backing, &stake, &vault, &index, &short); err != nil {
			return nil, err
		}
		out[id] = Backing{Backing: backing, Stake: uint64(stake), Position: client.PositionKey{Vault: vault, Index: uint32(index)}, RejectShort: uint64(short)}
	}
	return out, rows.Err()
}
