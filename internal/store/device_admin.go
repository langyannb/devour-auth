package store

import "context"

// ListDevices 设备列表，可按卡或指纹筛选。
func (s *Store) ListDevices(ctx context.Context, cardID int64, fingerprint string, includeUnbound bool, limit, offset int) ([]Device, int64, error) {
	where := " WHERE 1=1"
	var args []any
	if cardID > 0 {
		where += " AND card_id = ?"
		args = append(args, cardID)
	}
	if fingerprint != "" {
		where += " AND fingerprint LIKE ?"
		args = append(args, "%"+fingerprint+"%")
	}
	if !includeUnbound {
		where += " AND unbound_at IS NULL"
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM devices`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+deviceCols+` FROM devices`+where+` ORDER BY last_seen DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, d)
	}
	return out, total, rows.Err()
}

func (s *Store) SetDeviceBanned(ctx context.Context, id int64, banned bool) error {
	v := 0
	if banned {
		v = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE devices SET banned = ? WHERE id = ?`, v, id)
	return err
}

// UnbindDeviceByID 只解绑一台设备，不动其它绑定。
func (s *Store) UnbindDeviceByID(ctx context.Context, id, now int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE devices SET unbound_at = ? WHERE id = ? AND unbound_at IS NULL`, now, id)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// SessionRow 在线会话（带卡密与设备指纹，供后台展示）。
type SessionRow struct {
	ID            int64
	CardID        int64
	Kami          string
	DeviceID      int64
	Fingerprint   string
	AccessExp     int64
	RefreshExp    int64
	LastHeartbeat int64
	IP            string
	UA            string
	CreatedAt     int64
}

func (s *Store) ListSessions(ctx context.Context, now int64, limit, offset int) ([]SessionRow, int64, error) {
	return s.listSessions(ctx, true, now, 0, limit, offset)
}

func (s *Store) ListSessionsByCard(ctx context.Context, cardID int64, limit, offset int) ([]SessionRow, int64, error) {
	return s.listSessions(ctx, false, 0, cardID, limit, offset)
}

func (s *Store) listSessions(ctx context.Context, onlineOnly bool, now int64, cardID int64, limit, offset int) ([]SessionRow, int64, error) {
	where := " WHERE 1=1"
	var args []any
	if onlineOnly {
		where += " AND se.revoked_at IS NULL AND se.refresh_exp > ?"
		args = append(args, now)
	}
	if cardID > 0 {
		where += " AND se.card_id = ?"
		args = append(args, cardID)
	}

	var total int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions se`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT se.id, se.card_id, COALESCE(c.kami,''), se.device_id, COALESCE(d.fingerprint,''),
		        se.access_exp, se.refresh_exp, se.last_heartbeat, COALESCE(se.ip,''), COALESCE(se.ua,''), se.created_at
		   FROM sessions se
		   LEFT JOIN cards c ON c.id = se.card_id
		   LEFT JOIN devices d ON d.id = se.device_id`+where+
			` ORDER BY se.last_heartbeat DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []SessionRow
	for rows.Next() {
		var r SessionRow
		if err := rows.Scan(&r.ID, &r.CardID, &r.Kami, &r.DeviceID, &r.Fingerprint,
			&r.AccessExp, &r.RefreshExp, &r.LastHeartbeat, &r.IP, &r.UA, &r.CreatedAt); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *Store) RevokeSessionByID(ctx context.Context, id, now int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, id)
	return err
}
