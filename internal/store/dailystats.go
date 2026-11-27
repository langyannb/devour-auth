package store

import "context"

type DailyStat struct {
	Day           string `json:"day"`
	Activations   int64  `json:"activations"`
	Logins        int64  `json:"logins"`
	UniqueDevices int64  `json:"unique_devices"`
	IssuedCents   int64  `json:"issued_cents"`
}

// RollupDay 把某一天的事件汇总进 daily_stats（可重复执行，覆盖当天）。
func (s *Store) RollupDay(ctx context.Context, day string, start, end int64) (DailyStat, error) {
	out := DailyStat{Day: day}

	queries := []struct {
		q   string
		arg []any
		dst *int64
	}{
		{`SELECT COUNT(*) FROM cards WHERE activated_at >= ? AND activated_at < ?`, []any{start, end}, &out.Activations},
		{`SELECT COUNT(*) FROM events WHERE kind = 'login' AND ts >= ? AND ts < ?`, []any{start, end}, &out.Logins},
		{`SELECT COUNT(DISTINCT device_id) FROM events WHERE kind = 'login' AND device_id IS NOT NULL AND ts >= ? AND ts < ?`, []any{start, end}, &out.UniqueDevices},
		{`SELECT COALESCE(SUM(p.price_cents), 0) FROM cards c JOIN policies p ON p.id = c.policy_id
		   WHERE c.created_at >= ? AND c.created_at < ?`, []any{start, end}, &out.IssuedCents},
	}
	for _, x := range queries {
		if err := s.db.QueryRowContext(ctx, x.q, x.arg...).Scan(x.dst); err != nil {
			return out, err
		}
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO daily_stats(day, activations, logins, unique_devices, errors, online_peak, issued_cents)
		 VALUES(?,?,?,?,0,0,?)
		 ON CONFLICT(day) DO UPDATE SET
		   activations = excluded.activations,
		   logins = excluded.logins,
		   unique_devices = excluded.unique_devices,
		   issued_cents = excluded.issued_cents`,
		out.Day, out.Activations, out.Logins, out.UniqueDevices, out.IssuedCents)
	return out, err
}

func (s *Store) ListDailyStats(ctx context.Context, sinceDay string) ([]DailyStat, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT day, activations, logins, unique_devices, issued_cents
		   FROM daily_stats WHERE day >= ? ORDER BY day`, sinceDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DailyStat
	for rows.Next() {
		var d DailyStat
		if err := rows.Scan(&d.Day, &d.Activations, &d.Logins, &d.UniqueDevices, &d.IssuedCents); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
