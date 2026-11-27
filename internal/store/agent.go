package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Agent struct {
	ID                int64
	AppID             int64
	Name              string
	ParentID          sql.NullInt64
	Level             int64
	DiscountBP        int64
	RebateBP          int64
	MonthlyQuotaCents int64 // 旧字段，保留兼容
	QuotaCents        int64 // 当前可用额度（-1 = 不限）
	PassHash          string
	TotpSecret        string
	Status            string // active | frozen
	CreatedAt         int64
	FrozenAt          sql.NullInt64
	LoginSlug         string // 专属登录链接的路径段，空=没有
	PasswordSet       int64  // 0 = 等下级通过链接设置密码
}

const agentCols = `id, app_id, name, parent_id, level, discount_bp, rebate_bp, monthly_quota_cents, quota_cents,
  pass_hash, COALESCE(totp_secret,''), status, created_at, frozen_at, COALESCE(login_slug,''), password_set`

func scanAgent(sc interface{ Scan(...any) error }) (Agent, error) {
	var a Agent
	err := sc.Scan(&a.ID, &a.AppID, &a.Name, &a.ParentID, &a.Level, &a.DiscountBP, &a.RebateBP,
		&a.MonthlyQuotaCents, &a.QuotaCents, &a.PassHash, &a.TotpSecret, &a.Status, &a.CreatedAt, &a.FrozenAt,
		&a.LoginSlug, &a.PasswordSet)
	return a, err
}

func (s *Store) CreateAgent(ctx context.Context, a Agent) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO agents(app_id, name, parent_id, level, discount_bp, rebate_bp, monthly_quota_cents, quota_cents,
		   pass_hash, totp_secret, status, created_at, login_slug, password_set)
		 VALUES(?,?,?,?,?,?,?,?,?,NULLIF(?, ''),?,?,?,?)`,
		a.AppID, a.Name, a.ParentID, a.Level, a.DiscountBP, a.RebateBP, a.MonthlyQuotaCents, a.QuotaCents,
		a.PassHash, a.TotpSecret, a.Status, a.CreatedAt, nullIfEmpty(a.LoginSlug), a.PasswordSet)
	if err != nil {
		if isUniqueViolation(err) {
			return 0, ErrConflict
		}
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AgentByID(ctx context.Context, id int64) (Agent, error) {
	a, err := scanAgent(s.db.QueryRowContext(ctx,
		`SELECT `+agentCols+` FROM agents WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	return a, err
}

func (s *Store) AgentByName(ctx context.Context, name string) (Agent, error) {
	a, err := scanAgent(s.db.QueryRowContext(ctx,
		`SELECT `+agentCols+` FROM agents WHERE name = ?`, name))
	if errors.Is(err, sql.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	return a, err
}

// ListAgents 返回代理；appID 传 0 表示不按应用过滤，parentID 传 0 表示不按父级过滤。
func (s *Store) ListAgents(ctx context.Context, appID, parentID int64) ([]Agent, error) {
	q := `SELECT ` + agentCols + ` FROM agents`
	var (
		args  []any
		where []string
	)
	if appID > 0 {
		where = append(where, "app_id = ?")
		args = append(args, appID)
	}
	if parentID > 0 {
		where = append(where, "parent_id = ?")
		args = append(args, parentID)
	}
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += ` ORDER BY id`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Agent
	for rows.Next() {
		a, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// ChildAgentIDs 返回自己 + 直接下级（两级体系里就是全部可见范围）。
func (s *Store) ChildAgentIDs(ctx context.Context, agentID int64) ([]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id FROM agents WHERE id = ? OR parent_id = ?`, agentID, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) UpdateAgentPricing(ctx context.Context, id, discountBP, rebateBP, quotaCents int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET discount_bp = ?, rebate_bp = ?, monthly_quota_cents = ?, quota_cents = ? WHERE id = ?`,
		discountBP, rebateBP, quotaCents, quotaCents, id)
	return err
}

func (s *Store) SetAgentStatus(ctx context.Context, id int64, status string, frozenAt sql.NullInt64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agents SET status = ?, frozen_at = ? WHERE id = ?`, status, frozenAt, id)
	return err
}

func (s *Store) SetAgentPassword(ctx context.Context, id int64, passHash string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agents SET pass_hash = ? WHERE id = ?`, passHash, id)
	return err
}

// ---------- 流水 ----------

type AgentLedgerRow struct {
	ID          int64
	AgentID     int64
	Kind        string
	AmountCents int64
	Ref         string
	CreatedAt   int64
}

func (s *Store) AddAgentLedgerTx(ctx context.Context, tx *sql.Tx, agentID int64, kind string, amount int64, ref string, now int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO agent_ledger(agent_id, kind, amount_cents, ref, created_at) VALUES(?,?,?,?,?)`,
		agentID, kind, amount, ref, now)
	return err
}

// MonthlyIssuedCents 本月已出卡应付总额，用于额度判断。
func (s *Store) MonthlyIssuedCentsTx(ctx context.Context, tx *sql.Tx, agentID, monthStart int64) (int64, error) {
	var total sql.NullInt64
	err := tx.QueryRowContext(ctx,
		`SELECT SUM(amount_cents) FROM agent_ledger
		  WHERE agent_id = ? AND kind = 'issue' AND created_at >= ?`, agentID, monthStart).Scan(&total)
	return total.Int64, err
}

func (s *Store) ListAgentLedger(ctx context.Context, agentID int64, limit, offset int) ([]AgentLedgerRow, int64, error) {
	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM agent_ledger WHERE agent_id = ?`, agentID).Scan(&total); err != nil {
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
		   FROM agent_ledger WHERE agent_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		agentID, limit, offset)
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

// ---------- 账单 ----------

type AgentBill struct {
	ID          int64
	AgentID     int64
	Month       string
	AmountCents int64
	Status      string
	SettledAt   sql.NullInt64
	Note        string
}

// BillingMonth 汇总某代理某月的 issue 流水，生成或更新 open 账单。
func (s *Store) UpsertAgentBill(ctx context.Context, agentID int64, month string, monthStart, monthEnd, now int64) (bool, error) {
	var amount sql.NullInt64
	if err := s.db.QueryRowContext(ctx,
		`SELECT SUM(amount_cents) FROM agent_ledger
		  WHERE agent_id = ? AND kind = 'issue' AND created_at >= ? AND created_at < ?`,
		agentID, monthStart, monthEnd).Scan(&amount); err != nil {
		return false, err
	}
	if !amount.Valid {
		return false, nil
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_bills(agent_id, month, amount_cents, status, created_at)
		 VALUES(?,?,?, 'open', ?)
		 ON CONFLICT(agent_id, month) DO UPDATE SET amount_cents = excluded.amount_cents
		 WHERE agent_bills.status = 'open'`,
		agentID, month, amount.Int64, now)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) ListAgentBills(ctx context.Context, agentID int64) ([]AgentBill, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, agent_id, month, amount_cents, status, settled_at, COALESCE(note,'')
		   FROM agent_bills WHERE agent_id = ? ORDER BY month DESC`, agentID)
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

func (s *Store) SettleAgentBill(ctx context.Context, billID int64, now int64, note string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_bills SET status = 'settled', settled_at = ?, note = ?
		  WHERE id = ? AND status = 'open'`, now, note, billID)
	return err
}

// ---------- 会话 ----------

type AgentSession struct {
	ID        int64
	AgentID   int64
	CSRF      string
	IP        string
	CreatedAt int64
	LastSeen  int64
	ExpiresAt int64
	Reauth    int64
}

func (s *Store) CreateAgentSession(ctx context.Context, agentID int64, tokenHash, csrf, ip, ua string, now, expires int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_sessions(agent_id, token_hash, csrf_token, ip, ua, created_at, last_seen, expires_at)
		 VALUES(?,?,?,?,?,?,?,?)`, agentID, tokenHash, csrf, ip, ua, now, now, expires)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AgentSessionByTokenHash(ctx context.Context, hash string) (AgentSession, error) {
	var (
		sess      AgentSession
		revokedAt sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, agent_id, csrf_token, COALESCE(ip,''), created_at, last_seen, expires_at, revoked_at
		   FROM agent_sessions WHERE token_hash = ?`, hash).
		Scan(&sess.ID, &sess.AgentID, &sess.CSRF, &sess.IP, &sess.CreatedAt, &sess.LastSeen,
			&sess.ExpiresAt, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AgentSession{}, ErrNotFound
	}
	if revokedAt.Valid {
		return AgentSession{}, ErrNotFound
	}
	return sess, err
}

func (s *Store) TouchAgentSession(ctx context.Context, id, now int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agent_sessions SET last_seen = ? WHERE id = ?`, now, id)
	return err
}

func (s *Store) RevokeAgentSession(ctx context.Context, id, now int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, id)
	return err
}

func (s *Store) RevokeOtherAgentSessions(ctx context.Context, agentID, keepID, now int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_sessions SET revoked_at = ?
		  WHERE agent_id = ? AND id != ? AND revoked_at IS NULL`, now, agentID, keepID)
	return err
}

func nullIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// AgentBySlug 按专属链接标识找代理。
func (s *Store) AgentBySlug(ctx context.Context, slug string) (Agent, error) {
	a, err := scanAgent(s.db.QueryRowContext(ctx,
		`SELECT `+agentCols+` FROM agents WHERE login_slug = ?`, slug))
	if errors.Is(err, sql.ErrNoRows) {
		return Agent{}, ErrNotFound
	}
	return a, err
}

// ActivateAgentBySlug 下级通过专属链接首次设置密码；已设置过返回 ErrConflict。
func (s *Store) ActivateAgentBySlug(ctx context.Context, slug, passHash string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET pass_hash = ?, password_set = 1 WHERE login_slug = ? AND password_set = 0`,
		passHash, slug)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrConflict
	}
	return nil
}

// SetAgentLoginSlug 写专属链接标识。
func (s *Store) SetAgentLoginSlug(ctx context.Context, id int64, slug string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE agents SET login_slug = ? WHERE id = ?`, slug, id)
	return err
}
