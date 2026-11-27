package store

import (
	"context"
	"database/sql"
)

// CardEntitlementRow 单卡权限覆盖：revoked 为真表示显式撤销，否则是附加。
type CardEntitlementRow struct {
	Code      string
	Revoked   bool
	ExpiresAt int64
}

func (s *Store) CardEntitlementRowsTx(ctx context.Context, tx *sql.Tx, cardID int64) ([]CardEntitlementRow, error) {
	rows, err := tx.QueryContext(ctx,
		`SELECT e.code, ce.revoked_at IS NOT NULL, ce.expires_at
		   FROM card_entitlements ce
		   JOIN entitlements e ON e.id = ce.entitlement_id
		  WHERE ce.card_id = ?`, cardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CardEntitlementRow
	for rows.Next() {
		var r CardEntitlementRow
		if err := rows.Scan(&r.Code, &r.Revoked, &r.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GrantCardEntitlementTx 附加权限，可选独立到期（0 = 跟随卡）。
func (s *Store) GrantCardEntitlementTx(ctx context.Context, tx *sql.Tx,
	cardID, entitlementID, grantedBy, expiresAt int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO card_entitlements(card_id, entitlement_id, expires_at, granted_by, revoked_at)
		 VALUES(?,?,?,?,NULL)
		 ON CONFLICT(card_id, entitlement_id) DO UPDATE SET
		   expires_at = excluded.expires_at,
		   granted_by = excluded.granted_by,
		   revoked_at = NULL`,
		cardID, entitlementID, expiresAt, grantedBy)
	return err
}

// RevokeCardEntitlementTx 显式撤销；即使卡类型包含该权限也不再下发。
func (s *Store) RevokeCardEntitlementTx(ctx context.Context, tx *sql.Tx,
	cardID, entitlementID, grantedBy, now int64) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO card_entitlements(card_id, entitlement_id, expires_at, granted_by, revoked_at)
		 VALUES(?,?,0,?,?)
		 ON CONFLICT(card_id, entitlement_id) DO UPDATE SET
		   revoked_at = excluded.revoked_at,
		   granted_by = excluded.granted_by`,
		cardID, entitlementID, grantedBy, now)
	return err
}

func (s *Store) EntitlementByIDTx(ctx context.Context, tx *sql.Tx, id int64) (Entitlement, error) {
	var e Entitlement
	err := tx.QueryRowContext(ctx,
		`SELECT id, app_id, code, name, cost_points, cost_times, active, created_at
		   FROM entitlements WHERE id = ?`, id).
		Scan(&e.ID, &e.AppID, &e.Code, &e.Name, &e.CostPoints, &e.CostTimes, &e.Active, &e.CreatedAt)
	return e, err
}

func (s *Store) EntitlementByCodeTx(ctx context.Context, tx *sql.Tx, appID int64, code string) (Entitlement, error) {
	var e Entitlement
	err := tx.QueryRowContext(ctx,
		`SELECT id, app_id, code, name, cost_points, cost_times, active, created_at
		   FROM entitlements WHERE app_id = ? AND code = ?`, appID, code).
		Scan(&e.ID, &e.AppID, &e.Code, &e.Name, &e.CostPoints, &e.CostTimes, &e.Active, &e.CreatedAt)
	return e, err
}
