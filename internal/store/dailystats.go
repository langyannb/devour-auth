package store

import (
	"context"
	"sort"
)

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

// AppDailyTrend 指定应用按天的激活/登录/出卡金额。
// daily_stats 是全局汇总表，绑定应用的账号看板单独算，避免看到别人的数。
func (s *Store) AppDailyTrend(ctx context.Context, appID, since int64) ([]DailyStat, error) {
	byDay := map[string]*DailyStat{}
	get := func(day string) *DailyStat {
		d := byDay[day]
		if d == nil {
			d = &DailyStat{Day: day}
			byDay[day] = d
		}
		return d
	}
	scan := func(q string, fn func(*DailyStat, int64)) error {
		rows, err := s.db.QueryContext(ctx, q, appID, since)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var day string
			var n int64
			if err := rows.Scan(&day, &n); err != nil {
				return err
			}
			fn(get(day), n)
		}
		return rows.Err()
	}
	if err := scan(`SELECT date(activated_at, 'unixepoch', 'localtime') d, COUNT(*) FROM cards
	                 WHERE app_id = ? AND activated_at >= ? GROUP BY d`,
		func(d *DailyStat, n int64) { d.Activations = n }); err != nil {
		return nil, err
	}
	if err := scan(`SELECT date(e.ts, 'unixepoch', 'localtime') d, COUNT(*) FROM events e
	                  JOIN cards c ON c.id = e.card_id
	                 WHERE e.kind = 'login' AND c.app_id = ? AND e.ts >= ? GROUP BY d`,
		func(d *DailyStat, n int64) { d.Logins = n }); err != nil {
		return nil, err
	}
	if err := scan(`SELECT date(c.created_at, 'unixepoch', 'localtime') d, COALESCE(SUM(p.price_cents), 0)
	                  FROM cards c JOIN policies p ON p.id = c.policy_id
	                 WHERE c.app_id = ? AND c.created_at >= ? GROUP BY d`,
		func(d *DailyStat, n int64) { d.IssuedCents = n }); err != nil {
		return nil, err
	}
	out := make([]DailyStat, 0, len(byDay))
	for _, d := range byDay {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out, nil
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
