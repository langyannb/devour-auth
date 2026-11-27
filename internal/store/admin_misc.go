package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ---------- 黑名单 ----------

type BlacklistEntry struct {
	ID        int64
	AppID     int64
	Kind      string // ip | device | card
	Value     string
	Reason    string
	ExpiresAt int64
	CreatedAt int64
}

func (s *Store) ListBlacklist(ctx context.Context, appID int64) ([]BlacklistEntry, error) {
	q := `SELECT id, app_id, kind, value, COALESCE(reason,''), expires_at, created_at FROM blacklist`
	var args []any
	if appID > 0 {
		q += ` WHERE app_id = ? OR app_id = 0`
		args = append(args, appID)
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []BlacklistEntry
	for rows.Next() {
		var b BlacklistEntry
		if err := rows.Scan(&b.ID, &b.AppID, &b.Kind, &b.Value, &b.Reason, &b.ExpiresAt, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) AddBlacklist(ctx context.Context, b BlacklistEntry, createdBy int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO blacklist(app_id, kind, value, reason, expires_at, created_by, created_at)
		 VALUES(?,?,?,?,?,?,?)
		 ON CONFLICT(app_id, kind, value) DO UPDATE SET
		   reason = excluded.reason, expires_at = excluded.expires_at`,
		b.AppID, b.Kind, b.Value, b.Reason, b.ExpiresAt, createdBy, b.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) RemoveBlacklist(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM blacklist WHERE id = ?`, id)
	return err
}

// Blacklisted 判断是否命中（app_id 为 0 的条目对所有应用生效）。
func (s *Store) Blacklisted(ctx context.Context, appID int64, kind, value string, now int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM blacklist
		  WHERE kind = ? AND value = ? AND (app_id = 0 OR app_id = ?)
		    AND (expires_at = 0 OR expires_at > ?)`,
		kind, value, appID, now).Scan(&n)
	return n > 0, err
}

func (s *Store) BlacklistedTx(ctx context.Context, tx *sql.Tx, appID int64, kind, value string, now int64) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM blacklist
		  WHERE kind = ? AND value = ? AND (app_id = 0 OR app_id = ?)
		    AND (expires_at = 0 OR expires_at > ?)`,
		kind, value, appID, now).Scan(&n)
	return n > 0, err
}

// ---------- 事件日志 ----------

func (s *Store) ListEvents(ctx context.Context, appID, cardID int64, kind string, limit, offset int) ([]Event, int64, error) {
	where := ` WHERE 1=1`
	var args []any
	if appID > 0 {
		where += ` AND card_id IN (SELECT id FROM cards WHERE app_id = ?)`
		args = append(args, appID)
	}
	if cardID > 0 {
		where += ` AND card_id = ?`
		args = append(args, cardID)
	}
	if kind != "" {
		where += ` AND kind = ?`
		args = append(args, kind)
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM events`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT ts, kind, COALESCE(card_id,0), COALESCE(device_id,0), COALESCE(ip,''),
		        COALESCE(ua,''), COALESCE(code,''), COALESCE(detail,'')
		   FROM events`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Event
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.TS, &e.Kind, &e.CardID, &e.DeviceID, &e.IP, &e.UA, &e.Code, &e.Detail); err != nil {
			return nil, 0, err
		}
		out = append(out, e)
	}
	return out, total, rows.Err()
}

// ---------- 财务 ----------

func (s *Store) ListAllLedger(ctx context.Context, appID, agentID int64, limit, offset int) ([]AgentLedgerRow, int64, error) {
	where := ""
	var args []any
	conds := []string{}
	if agentID > 0 {
		conds = append(conds, "agent_id = ?")
		args = append(args, agentID)
	}
	if appID > 0 {
		conds = append(conds, "agent_id IN (SELECT id FROM agents WHERE app_id = ?)")
		args = append(args, appID)
	}
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_ledger`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, kind, amount_cents, COALESCE(ref,''), created_at
		   FROM agent_ledger`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []AgentLedgerRow
	for rows.Next() {
		var r AgentLedgerRow
		if err := rows.Scan(&r.ID, &r.AgentID, &r.Kind, &r.AmountCents, &r.Ref, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) ListAllBills(ctx context.Context, appID int64, status string) ([]AgentBill, error) {
	q := `SELECT id, agent_id, month, amount_cents, status, settled_at, COALESCE(note,'') FROM agent_bills`
	conds := []string{}
	var args []any
	if status != "" {
		conds = append(conds, "status = ?")
		args = append(args, status)
	}
	if appID > 0 {
		conds = append(conds, "agent_id IN (SELECT id FROM agents WHERE app_id = ?)")
		args = append(args, appID)
	}
	if len(conds) > 0 {
		q += " WHERE " + strings.Join(conds, " AND ")
	}
	q += ` ORDER BY month DESC, id DESC`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentBill
	for rows.Next() {
		var b AgentBill
		if err := rows.Scan(&b.ID, &b.AgentID, &b.Month, &b.AmountCents, &b.Status, &b.SettledAt, &b.Note); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---------- 卡密批次 ----------

type Batch struct {
	ID        int64
	AppID     int64
	PolicyID  int64
	AgentID   sql.NullInt64
	Qty       int64
	CreatedBy int64
	CreatedAt int64
}

func (s *Store) ListBatches(ctx context.Context, appID int64) ([]Batch, error) {
	q := `SELECT id, app_id, policy_id, agent_id, qty, created_by, created_at FROM batches`
	var args []any
	if appID > 0 {
		q += ` WHERE app_id = ?`
		args = append(args, appID)
	}
	q += ` ORDER BY id DESC LIMIT 200`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Batch
	for rows.Next() {
		var b Batch
		if err := rows.Scan(&b.ID, &b.AppID, &b.PolicyID, &b.AgentID, &b.Qty, &b.CreatedBy, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

var _ = errors.Is
