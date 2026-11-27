package store

import (
	"context"
	"database/sql"
)

// SetAgentPrice 给某个代理的某个卡类型定价（覆盖，0 表示删掉回到默认折扣价）。
func (s *Store) SetAgentPrice(ctx context.Context, agentID, policyID, priceCents, now int64) error {
	if priceCents <= 0 {
		_, err := s.db.ExecContext(ctx,
			`DELETE FROM agent_prices WHERE agent_id = ? AND policy_id = ?`, agentID, policyID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO agent_prices(agent_id, policy_id, price_cents, updated_at)
		 VALUES(?,?,?,?)
		 ON CONFLICT(agent_id, policy_id) DO UPDATE
		   SET price_cents = excluded.price_cents, updated_at = excluded.updated_at`,
		agentID, policyID, priceCents, now)
	return err
}

func (s *Store) ListAgentPrices(ctx context.Context, agentID int64) (map[int64]int64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT policy_id, price_cents FROM agent_prices WHERE agent_id = ?`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int64{}
	for rows.Next() {
		var pid, price int64
		if err := rows.Scan(&pid, &price); err != nil {
			return nil, err
		}
		out[pid] = price
	}
	return out, rows.Err()
}

// AgentPriceFor 取代理某卡类型的单独定价；没有则返回 false。
func (s *Store) AgentPriceFor(ctx context.Context, agentID, policyID int64) (int64, bool, error) {
	var price int64
	err := s.db.QueryRowContext(ctx,
		`SELECT price_cents FROM agent_prices WHERE agent_id = ? AND policy_id = ?`,
		agentID, policyID).Scan(&price)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return price, true, nil
}
