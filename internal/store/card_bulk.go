package store

import (
	"context"
	"database/sql"
	"fmt"
)

// cardScopeWhere 把界面上的范围换成 SQL 片段：active / unused / expired / all。
func cardScopeWhere(scope string) (string, bool) {
	switch scope {
	case "active", "unused", "expired":
		return " AND status = '" + scope + "'", true
	case "all":
		return "", true
	}
	return "", false
}

// CountCardsByScope 返回该范围内总卡数与可调时的卡数（有 expire_at 的）。
func (s *Store) CountCardsByScope(ctx context.Context, appID int64, scope string) (total, timeCards int64, err error) {
	where, ok := cardScopeWhere(scope)
	if !ok {
		return 0, 0, fmt.Errorf("范围不合法")
	}
	q := `SELECT COUNT(*), COALESCE(SUM(CASE WHEN expire_at > 0 THEN 1 ELSE 0 END), 0)
	        FROM cards WHERE app_id = ?` + where
	if err := s.db.QueryRowContext(ctx, q, appID).Scan(&total, &timeCards); err != nil {
		return 0, 0, err
	}
	return total, timeCards, nil
}

// BulkAddExpire 给范围内所有有时长的卡加时；已过期的加时后恢复可用。
func (s *Store) BulkAddExpire(ctx context.Context, tx *sql.Tx, appID int64, scope string, seconds int64) (int64, error) {
	where, ok := cardScopeWhere(scope)
	if !ok {
		return 0, fmt.Errorf("范围不合法")
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE cards
		    SET expire_at = expire_at + ?,
		        status = CASE WHEN status = 'expired' THEN 'active' ELSE status END
		  WHERE app_id = ? AND expire_at > 0`+where, seconds, appID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BulkSubExpire 扣时；扣到当前时间或更早的卡标记为已过期。
func (s *Store) BulkSubExpire(ctx context.Context, tx *sql.Tx, appID int64, scope string, seconds, now int64) (int64, error) {
	where, ok := cardScopeWhere(scope)
	if !ok {
		return 0, fmt.Errorf("范围不合法")
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE cards
		    SET expire_at = CASE WHEN expire_at - ? <= ? THEN ? ELSE expire_at - ? END,
		        status = CASE WHEN expire_at - ? <= ? AND status IN ('active', 'unused')
		                      THEN 'expired' ELSE status END
		  WHERE app_id = ? AND expire_at > 0`+where,
		seconds, now, now, seconds, seconds, now, appID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// BulkRecycleExpired 清理所有已过期卡（软删，置为 recycled）。
func (s *Store) BulkRecycleExpired(ctx context.Context, tx *sql.Tx, appID int64) (int64, error) {
	res, err := tx.ExecContext(ctx,
		`UPDATE cards SET status = 'recycled' WHERE app_id = ? AND status = 'expired'`, appID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
