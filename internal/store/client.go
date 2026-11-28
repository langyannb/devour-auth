package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// ---------- 设备 ----------

type Device struct {
	ID          int64
	CardID      int64
	Fingerprint string
	Model       string
	Brand       string
	OS          string
	AppVersion  string
	FirstSeen   int64
	LastSeen    int64
	Banned      int64
	UnboundAt   sql.NullInt64
}

const deviceCols = `id, card_id, fingerprint, COALESCE(model,''), COALESCE(brand,''),
  COALESCE(os,''), COALESCE(app_version,''), first_seen, last_seen, banned, unbound_at`

func scanDevice(sc interface{ Scan(...any) error }) (Device, error) {
	var d Device
	err := sc.Scan(&d.ID, &d.CardID, &d.Fingerprint, &d.Model, &d.Brand,
		&d.OS, &d.AppVersion, &d.FirstSeen, &d.LastSeen, &d.Banned, &d.UnboundAt)
	return d, err
}

// BoundDeviceTx 取该卡当前仍绑定的设备。
func (s *Store) BoundDeviceTx(ctx context.Context, tx *sql.Tx, cardID int64, fingerprint string) (Device, error) {
	d, err := scanDevice(tx.QueryRowContext(ctx,
		`SELECT `+deviceCols+` FROM devices
		  WHERE card_id = ? AND fingerprint = ? AND unbound_at IS NULL`, cardID, fingerprint))
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

func (s *Store) CountBoundDevicesTx(ctx context.Context, tx *sql.Tx, cardID int64) (int64, error) {
	var n int64
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM devices WHERE card_id = ? AND unbound_at IS NULL`, cardID).Scan(&n)
	return n, err
}

// RebindDeviceTx 重新启用之前解绑过的同一台设备：设备行不重复插入
// （devices 上有 (card_id, fingerprint) 唯一约束），只清掉 unbound_at。
func (s *Store) RebindDeviceTx(ctx context.Context, tx *sql.Tx, cardID int64, fingerprint string, now int64) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`UPDATE devices SET unbound_at = NULL, last_seen = ?
		  WHERE card_id = ? AND fingerprint = ? AND unbound_at IS NOT NULL`,
		now, cardID, fingerprint)
	if err != nil {
		return 0, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return 0, err
	} else if n == 0 {
		return 0, ErrNotFound
	}
	var id int64
	err = tx.QueryRowContext(ctx,
		`SELECT id FROM devices WHERE card_id = ? AND fingerprint = ?`,
		cardID, fingerprint).Scan(&id)
	return id, err
}

func (s *Store) InsertDeviceTx(ctx context.Context, tx *sql.Tx, d Device) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO devices(card_id, fingerprint, model, brand, os, app_version, first_seen, last_seen, banned)
		 VALUES(?,?,?,?,?,?,?,?,0)`,
		d.CardID, d.Fingerprint, d.Model, d.Brand, d.OS, d.AppVersion, d.FirstSeen, d.LastSeen)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) TouchDeviceTx(ctx context.Context, tx *sql.Tx, id, now int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE devices SET last_seen = ? WHERE id = ?`, now, id)
	return err
}

// SessionCardApp 会话所属卡的应用，用于限制跨应用操作。
func (s *Store) SessionCardApp(ctx context.Context, sessionID int64) (int64, error) {
	var app int64
	err := s.db.QueryRowContext(ctx,
		`SELECT c.app_id FROM sessions se JOIN cards c ON c.id = se.card_id WHERE se.id = ?`, sessionID).Scan(&app)
	return app, err
}

func (s *Store) DeviceByID(ctx context.Context, id int64) (Device, error) {
	d, err := scanDevice(s.db.QueryRowContext(ctx,
		`SELECT `+deviceCols+` FROM devices WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

func (s *Store) DeviceByIDTx(ctx context.Context, tx *sql.Tx, id int64) (Device, error) {
	d, err := scanDevice(tx.QueryRowContext(ctx,
		`SELECT `+deviceCols+` FROM devices WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return Device{}, ErrNotFound
	}
	return d, err
}

// ---------- 客户端会话 ----------

type ClientSession struct {
	ID            int64
	CardID        int64
	DeviceID      int64
	AccessHash    string
	RefreshHash   string
	AccessExp     int64
	RefreshExp    int64
	LastHeartbeat int64
	ReuseDetected int64
	RevokedAt     sql.NullInt64
	IP            string
	UA            string
	CreatedAt     int64
}

const clientSessionCols = `id, card_id, device_id, access_hash, refresh_hash,
  access_exp, refresh_exp, last_heartbeat, reuse_detected, revoked_at,
  COALESCE(ip,''), COALESCE(ua,''), created_at`

func scanClientSession(sc interface{ Scan(...any) error }) (ClientSession, error) {
	var c ClientSession
	err := sc.Scan(&c.ID, &c.CardID, &c.DeviceID, &c.AccessHash, &c.RefreshHash,
		&c.AccessExp, &c.RefreshExp, &c.LastHeartbeat, &c.ReuseDetected, &c.RevokedAt,
		&c.IP, &c.UA, &c.CreatedAt)
	return c, err
}

func (s *Store) CreateClientSessionTx(ctx context.Context, tx *sql.Tx, c ClientSession) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`INSERT INTO sessions(card_id, device_id, access_hash, refresh_hash,
		   access_exp, refresh_exp, last_heartbeat, ip, ua, created_at)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		c.CardID, c.DeviceID, c.AccessHash, c.RefreshHash,
		c.AccessExp, c.RefreshExp, c.LastHeartbeat, c.IP, c.UA, c.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ActiveClientSessionsTx 未吊销且 refresh 未过期的会话，最旧在前（用于挤下线）。
func (s *Store) ActiveClientSessionsTx(ctx context.Context, tx *sql.Tx, cardID, now int64) ([]ClientSession, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT `+clientSessionCols+` FROM sessions
		  WHERE card_id = ? AND revoked_at IS NULL AND refresh_exp > ?
		  ORDER BY created_at, id`, cardID, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientSession
	for rows.Next() {
		c, err := scanClientSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) RevokeClientSessionTx(ctx context.Context, tx *sql.Tx, id, now int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, id)
	return err
}

func (s *Store) RevokeAllClientSessionsTx(ctx context.Context, tx *sql.Tx, cardID, now int64) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE card_id = ? AND revoked_at IS NULL`, now, cardID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) ClientSessionByAccessHash(ctx context.Context, hash string) (ClientSession, error) {
	c, err := scanClientSession(s.db.QueryRowContext(ctx,
		`SELECT `+clientSessionCols+` FROM sessions WHERE access_hash = ?`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return ClientSession{}, ErrNotFound
	}
	return c, err
}

func (s *Store) ClientSessionByRefreshHash(ctx context.Context, hash string) (ClientSession, error) {
	c, err := scanClientSession(s.db.QueryRowContext(ctx,
		`SELECT `+clientSessionCols+` FROM sessions WHERE refresh_hash = ?`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return ClientSession{}, ErrNotFound
	}
	return c, err
}

func (s *Store) ClientSessionByRefreshHashTx(ctx context.Context, tx *sql.Tx, hash string) (ClientSession, error) {
	c, err := scanClientSession(tx.QueryRowContext(ctx,
		`SELECT `+clientSessionCols+` FROM sessions WHERE refresh_hash = ?`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return ClientSession{}, ErrNotFound
	}
	return c, err
}

func (s *Store) ClientSessionByAccessHashTx(ctx context.Context, tx *sql.Tx, hash string) (ClientSession, error) {
	c, err := scanClientSession(tx.QueryRowContext(ctx,
		`SELECT `+clientSessionCols+` FROM sessions WHERE access_hash = ?`, hash))
	if errors.Is(err, sql.ErrNoRows) {
		return ClientSession{}, ErrNotFound
	}
	return c, err
}

// RotateTokens 同时换掉 access 与 refresh。
func (s *Store) RotateTokens(ctx context.Context, tx *sql.Tx, id int64,
	accessHash string, accessExp int64, refreshHash string, refreshExp int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE sessions SET access_hash = ?, access_exp = ?, refresh_hash = ?, refresh_exp = ?
		  WHERE id = ?`, accessHash, accessExp, refreshHash, refreshExp, id)
	return err
}

func (s *Store) TouchClientSession(ctx context.Context, id, now int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_heartbeat = ? WHERE id = ?`, now, id)
	return err
}

func (s *Store) TouchClientSessionTx(ctx context.Context, tx *sql.Tx, id, now int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE sessions SET last_heartbeat = ? WHERE id = ?`, now, id)
	return err
}

// RotateRefresh 换新 refresh，旧哈希立即失效。
func (s *Store) RotateRefresh(ctx context.Context, tx *sql.Tx, id int64, refreshHash string, refreshExp int64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE sessions SET refresh_hash = ?, refresh_exp = ? WHERE id = ?`,
		refreshHash, refreshExp, id)
	return err
}

// MarkReuse 标记盗用并整卡吊销。
func (s *Store) MarkReuse(ctx context.Context, tx *sql.Tx, id, cardID, now int64) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE sessions SET reuse_detected = 1 WHERE id = ?`, id); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx,
		`UPDATE sessions SET revoked_at = ? WHERE card_id = ? AND revoked_at IS NULL`, now, cardID)
	return err
}

// ---------- 事件 ----------

type Event struct {
	TS       int64
	Kind     string
	CardID   int64
	DeviceID int64
	IP       string
	UA       string
	Code     string
	Detail   string
}

func (s *Store) InsertEventTx(ctx context.Context, tx *sql.Tx, e Event) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO events(ts, kind, card_id, device_id, ip, ua, code, detail)
		 VALUES(?,?,NULLIF(?,0),NULLIF(?,0),?,?,?,?)`,
		e.TS, e.Kind, e.CardID, e.DeviceID, e.IP, e.UA, e.Code, e.Detail)
	return err
}

// ---------- 客户端认证辅助 ----------

// ClaimTrialTx 抢占试用名额；(app_id, fingerprint) 唯一，已被领过返回 ErrConflict。
func (s *Store) ClaimTrialTx(ctx context.Context, tx *sql.Tx, appID int64, fingerprint string, cardID, now int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO trial_claims(app_id, fingerprint, card_id, claimed_at) VALUES(?,?,?,?)`,
		appID, fingerprint, cardID, now)
	if err != nil && isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

func (s *Store) TrialClaimedTx(ctx context.Context, tx *sql.Tx, appID int64, fingerprint string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM trial_claims WHERE app_id = ? AND fingerprint = ?`,
		appID, fingerprint).Scan(&n)
	return n > 0, err
}

// RecentSessionForDeviceTx 判断该设备在窗口内是否已有会话（次数卡重连不重复扣）。
func (s *Store) RecentSessionForDeviceTx(ctx context.Context, tx *sql.Tx, cardID, deviceID, since int64) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions
		  WHERE card_id = ? AND device_id = ? AND created_at >= ?`, cardID, deviceID, since).Scan(&n)
	return n > 0, err
}

func (s *Store) InsertRefreshHistoryTx(ctx context.Context, tx *sql.Tx, hash string, sessionID, cardID, now int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO refresh_history(hash, session_id, card_id, rotated_at) VALUES(?,?,?,?)`,
		hash, sessionID, cardID, now)
	return err
}

// RefreshReuseCardTx 旧 refresh 是否已被轮换过；返回对应卡 id。
func (s *Store) RefreshReuseCardTx(ctx context.Context, tx *sql.Tx, hash string) (int64, bool, error) {
	var cardID int64
	err := tx.QueryRowContext(ctx, `SELECT card_id FROM refresh_history WHERE hash = ?`, hash).Scan(&cardID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return cardID, true, err
}

// EntitlementsByCodesTx 取仍然启用的权限定义，供登录下发实时定价。
func (s *Store) EntitlementsByCodesTx(ctx context.Context, tx *sql.Tx, appID int64, codes []string) ([]Entitlement, error) {
	if len(codes) == 0 {
		return nil, nil
	}
	placeholders := make([]byte, 0, len(codes)*2)
	args := make([]any, 0, len(codes)+1)
	args = append(args, appID)
	for i, c := range codes {
		if i > 0 {
			placeholders = append(placeholders, ',')
		}
		placeholders = append(placeholders, '?')
		args = append(args, c)
	}
	rows, err := tx.QueryContext(ctx,
		`SELECT id, app_id, code, name, cost_points, cost_times, active, created_at
		   FROM entitlements
		  WHERE app_id = ? AND active = 1 AND code IN (`+string(placeholders)+`)
		  ORDER BY code`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entitlement
	for rows.Next() {
		var e Entitlement
		if err := rows.Scan(&e.ID, &e.AppID, &e.Code, &e.Name, &e.CostPoints, &e.CostTimes, &e.Active, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}

// LastSessionIPByCardTx 该卡最近一次会话的 IP，用于换 IP 异常记录。
func (s *Store) LastSessionIPByCardTx(ctx context.Context, tx *sql.Tx, cardID int64) (string, error) {
	var ip sql.NullString
	err := tx.QueryRowContext(ctx,
		`SELECT ip FROM sessions WHERE card_id = ? ORDER BY id DESC LIMIT 1`, cardID).Scan(&ip)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return ip.String, nil
}
