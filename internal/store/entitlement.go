package store

import (
	"context"
	"database/sql"
	"errors"
)

type Entitlement struct {
	ID         int64
	AppID      int64
	Code       string
	Name       string
	CostPoints int64
	CostTimes  int64
	Active     int64
	CreatedAt  int64
}

func (s *Store) CreateEntitlement(ctx context.Context, e Entitlement) (int64, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO entitlements(app_id, code, name, cost_points, cost_times, active, created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		e.AppID, e.Code, e.Name, e.CostPoints, e.CostTimes, e.Active, e.CreatedAt)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) EntitlementByCode(ctx context.Context, appID int64, code string) (Entitlement, error) {
	var e Entitlement
	err := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, code, name, cost_points, cost_times, active, created_at
		   FROM entitlements WHERE app_id = ? AND code = ?`, appID, code).
		Scan(&e.ID, &e.AppID, &e.Code, &e.Name, &e.CostPoints, &e.CostTimes, &e.Active, &e.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Entitlement{}, ErrNotFound
	}
	return e, err
}

func (s *Store) ListEntitlements(ctx context.Context, appID int64) ([]Entitlement, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, app_id, code, name, cost_points, cost_times, active, created_at
		   FROM entitlements WHERE app_id = ? ORDER BY code`, appID)
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

// SetPolicyEntitlements 覆盖某个卡类型的权限集。
func (s *Store) SetPolicyEntitlements(ctx context.Context, policyID int64, entitlementIDs []int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM policy_entitlements WHERE policy_id = ?`, policyID); err != nil {
		return err
	}
	for _, eid := range entitlementIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO policy_entitlements(policy_id, entitlement_id) VALUES(?, ?)`,
			policyID, eid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CountPoliciesByApp(ctx context.Context, appID int64) (int64, error) {
	var n int64
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM policies WHERE app_id = ?`, appID).Scan(&n)
	return n, err
}
