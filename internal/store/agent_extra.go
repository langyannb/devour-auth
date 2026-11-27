package store

import "context"

// MonthlyIssuedCents 某代理本月已出卡应付总额（非事务版）。
func (s *Store) MonthlyIssuedCents(ctx context.Context, agentID, monthStart int64) (int64, error) {
	var total int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(amount_cents),0) FROM agent_ledger
		  WHERE agent_id = ? AND kind = 'issue' AND created_at >= ?`, agentID, monthStart).Scan(&total)
	return total, err
}

func (s *Store) CountCardsByAgents(ctx context.Context, agentIDs []int64) (int64, error) {
	if len(agentIDs) == 0 {
		return 0, nil
	}
	q := `SELECT COUNT(*) FROM cards WHERE agent_id IN (?` + repeatComma(len(agentIDs)-1) + `)`
	args := make([]any, len(agentIDs))
	for i, id := range agentIDs {
		args[i] = id
	}
	var n int64
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

func (s *Store) CountSessionsByAgents(ctx context.Context, agentIDs []int64, now int64) (int64, error) {
	if len(agentIDs) == 0 {
		return 0, nil
	}
	q := `SELECT COUNT(*) FROM sessions se JOIN cards c ON c.id = se.card_id
	       WHERE se.revoked_at IS NULL AND se.refresh_exp > ?
	         AND c.agent_id IN (?` + repeatComma(len(agentIDs)-1) + `)`
	args := []any{now}
	for _, id := range agentIDs {
		args = append(args, id)
	}
	var n int64
	err := s.db.QueryRowContext(ctx, q, args...).Scan(&n)
	return n, err
}

func repeatComma(n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += ",?"
	}
	return out
}

// AddAgentQuota 加减额度（正数加、负数扣），返回新余额。
func (s *Store) AddAgentQuota(ctx context.Context, agentID, delta int64) (int64, error) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE agents SET quota_cents = quota_cents + ? WHERE id = ?`, delta, agentID); err != nil {
		return 0, err
	}
	var v int64
	err := s.db.QueryRowContext(ctx, `SELECT quota_cents FROM agents WHERE id = ?`, agentID).Scan(&v)
	return v, err
}

// DeductAgentQuota 原子扣额度；不足返回 false。quota=-1 表示不限，不扣。
func (s *Store) DeductAgentQuota(ctx context.Context, agentID, cost int64) (int64, bool, error) {
	var quota int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT quota_cents FROM agents WHERE id = ?`, agentID).Scan(&quota); err != nil {
		return 0, false, err
	}
	if quota < 0 {
		return quota, true, nil // 不限
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE agents SET quota_cents = quota_cents - ? WHERE id = ? AND quota_cents >= ?`,
		cost, agentID, cost)
	if err != nil {
		return 0, false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return quota, false, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT quota_cents FROM agents WHERE id = ?`, agentID).Scan(&quota)
	return quota, err == nil, err
}
