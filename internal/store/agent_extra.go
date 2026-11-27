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
