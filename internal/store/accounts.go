package store

import (
	"context"
	"database/sql"
	"errors"
)

type AdminAccount struct {
	ID          int64
	Username    string
	Role        string
	TotpEnabled bool
	IPAllow     string
	LockedUntil int64
	CreatedAt   int64
	UnusedCodes int64
	AppID       int64
}

const adminAccountCols = `a.id, a.username, a.role, a.totp_secret IS NOT NULL, COALESCE(a.ip_allow, ''), a.locked_until, a.created_at,
	(SELECT COUNT(*) FROM admin_recovery_codes r WHERE r.admin_id = a.id AND r.used_at IS NULL), a.app_id`

func (s *Store) ListAdminAccounts(ctx context.Context) ([]AdminAccount, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+adminAccountCols+` FROM admins a ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AdminAccount
	for rows.Next() {
		var a AdminAccount
		if err := rows.Scan(&a.ID, &a.Username, &a.Role, &a.TotpEnabled, &a.IPAllow,
			&a.LockedUntil, &a.CreatedAt, &a.UnusedCodes, &a.AppID); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) AdminAccountByID(ctx context.Context, id int64) (AdminAccount, error) {
	var a AdminAccount
	err := s.db.QueryRowContext(ctx,
		`SELECT `+adminAccountCols+` FROM admins a WHERE a.id = ?`, id).
		Scan(&a.ID, &a.Username, &a.Role, &a.TotpEnabled, &a.IPAllow,
			&a.LockedUntil, &a.CreatedAt, &a.UnusedCodes, &a.AppID)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminAccount{}, ErrNotFound
	}
	return a, err
}

func (s *Store) AddRecoveryCodes(ctx context.Context, adminID int64, hashes []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, h := range hashes {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO admin_recovery_codes(admin_id, code_hash) VALUES(?,?)`, adminID, h); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UseRecoveryCode 消耗一个未用过的恢复码。
func (s *Store) UseRecoveryCode(ctx context.Context, adminID int64, hash string, now int64) (bool, error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE admin_recovery_codes SET used_at = ?
		  WHERE admin_id = ? AND code_hash = ? AND used_at IS NULL`, now, adminID, hash)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *Store) ClearRecoveryCodes(ctx context.Context, adminID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_recovery_codes WHERE admin_id = ?`, adminID)
	return err
}

var _ = sql.ErrNoRows
