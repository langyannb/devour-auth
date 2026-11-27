package store

import (
	"context"
	"database/sql"
	"errors"
)

// ErrNotFound 记录不存在。
var ErrNotFound = errors.New("not found")

// ErrConflict 唯一约束冲突。
var ErrConflict = errors.New("conflict")

type Admin struct {
	ID          int64
	Username    string
	PassHash    string
	Role        string // admin | operator
	TotpSecret  string // 空 = 未启用 TOTP
	IPAllow     string
	LockedUntil int64
}

func (s *Store) AdminByUsername(ctx context.Context, username string) (Admin, error) {
	var (
		a        Admin
		totp, ip sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, pass_hash, role, totp_secret, ip_allow, locked_until
		   FROM admins WHERE username = ?`, username).
		Scan(&a.ID, &a.Username, &a.PassHash, &a.Role, &totp, &ip, &a.LockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, err
	}
	a.TotpSecret, a.IPAllow = totp.String, ip.String
	return a, nil
}

func (s *Store) AdminByID(ctx context.Context, id int64) (Admin, error) {
	var (
		a        Admin
		totp, ip sql.NullString
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, username, pass_hash, role, totp_secret, ip_allow, locked_until
		   FROM admins WHERE id = ?`, id).
		Scan(&a.ID, &a.Username, &a.PassHash, &a.Role, &totp, &ip, &a.LockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		return Admin{}, ErrNotFound
	}
	if err != nil {
		return Admin{}, err
	}
	a.TotpSecret, a.IPAllow = totp.String, ip.String
	return a, nil
}

func (s *Store) AdminCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admins`).Scan(&n)
	return n, err
}

func (s *Store) CreateAdmin(ctx context.Context, username, passHash, role, totpSecret string, now int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO admins(username, pass_hash, role, totp_secret, created_at)
		 VALUES(?, ?, ?, NULLIF(?, ''), ?)`,
		username, passHash, role, totpSecret, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

type AdminSession struct {
	ID          int64
	AdminID     int64
	CSRF        string
	IP          string
	CreatedAt   int64
	LastSeen    int64
	ExpiresAt   int64
	ReauthUntil int64
}

func (s *Store) CreateAdminSession(ctx context.Context, adminID int64, tokenHash, csrf, ip, ua string, now, expires int64) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO admin_sessions(admin_id, token_hash, csrf_token, ip, ua, created_at, last_seen, expires_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		adminID, tokenHash, csrf, ip, ua, now, now, expires)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) AdminSessionByTokenHash(ctx context.Context, tokenHash string) (AdminSession, error) {
	var (
		sess      AdminSession
		revokedAt sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx,
		`SELECT id, admin_id, csrf_token, COALESCE(ip,''), created_at, last_seen, expires_at, reauth_until, revoked_at
		   FROM admin_sessions WHERE token_hash = ?`, tokenHash).
		Scan(&sess.ID, &sess.AdminID, &sess.CSRF, &sess.IP, &sess.CreatedAt, &sess.LastSeen,
			&sess.ExpiresAt, &sess.ReauthUntil, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminSession{}, ErrNotFound
	}
	if err != nil {
		return AdminSession{}, err
	}
	if revokedAt.Valid {
		return AdminSession{}, ErrNotFound
	}
	return sess, nil
}

func (s *Store) TouchAdminSession(ctx context.Context, id, now int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE admin_sessions SET last_seen = ? WHERE id = ?`, now, id)
	return err
}

func (s *Store) RevokeAdminSession(ctx context.Context, id, now int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admin_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, now, id)
	return err
}

// RevokeOtherAdminSessions 新登录踢掉该账号的旧会话（design.md 16.1）。
func (s *Store) RevokeOtherAdminSessions(ctx context.Context, adminID, keepID, now int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admin_sessions SET revoked_at = ?
		  WHERE admin_id = ? AND id != ? AND revoked_at IS NULL`, now, adminID, keepID)
	return err
}

func (s *Store) SetAdminReauth(ctx context.Context, sessionID, until int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE admin_sessions SET reauth_until = ? WHERE id = ?`, until, sessionID)
	return err
}
