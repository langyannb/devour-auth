package store

import "context"

// Stats 概览页用的汇总数字。
type Stats struct {
	CardsTotal     int64
	CardsByStatus  map[string]int64
	DevicesBound   int64
	AgentsActive   int64
	ActivationsDay int64
	OnlineSessions int64
}

func (s *Store) Stats(ctx context.Context, appID, dayStart, now int64) (Stats, error) {
	out := Stats{CardsByStatus: map[string]int64{}}

	rows, err := s.db.QueryContext(ctx, `SELECT status, COUNT(*) FROM cards WHERE (? = 0 OR app_id = ?) GROUP BY status`, appID, appID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var st string
		var n int64
		if err := rows.Scan(&st, &n); err != nil {
			rows.Close()
			return out, err
		}
		out.CardsByStatus[st] = n
		out.CardsTotal += n
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}

	scalars := []struct {
		q   string
		arg []any
		dst *int64
	}{
		{`SELECT COUNT(*) FROM devices d JOIN cards c ON c.id = d.card_id
		   WHERE d.unbound_at IS NULL AND (? = 0 OR c.app_id = ?)`, []any{appID, appID}, &out.DevicesBound},
		{`SELECT COUNT(*) FROM agents WHERE status = 'active' AND (? = 0 OR app_id = ?)`, []any{appID, appID}, &out.AgentsActive},
		{`SELECT COUNT(*) FROM cards WHERE activated_at >= ? AND (? = 0 OR app_id = ?)`,
			[]any{dayStart, appID, appID}, &out.ActivationsDay},
		{`SELECT COUNT(*) FROM sessions se JOIN cards c ON c.id = se.card_id
		   WHERE se.revoked_at IS NULL AND se.refresh_exp > ? AND (? = 0 OR c.app_id = ?)`,
			[]any{now, appID, appID}, &out.OnlineSessions},
	}
	for _, x := range scalars {
		if err := s.db.QueryRowContext(ctx, x.q, x.arg...).Scan(x.dst); err != nil {
			return out, err
		}
	}
	return out, nil
}
