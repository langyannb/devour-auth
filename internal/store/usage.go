package store

import (
	"context"
	"database/sql"
	"errors"
)

// Usage 一次权限消耗记录；nonce 全局唯一，用于幂等。
type Usage struct {
	ID           int64
	CardID       int64
	Code         string
	CostPoints   int64
	CostTimes    int64
	BalanceAfter int64
	Nonce        string
	CreatedAt    int64
}

func (s *Store) UsageByNonceTx(ctx context.Context, tx *sql.Tx, nonce string) (Usage, error) {
	var u Usage
	err := tx.QueryRowContext(ctx,
		`SELECT id, card_id, code, cost_points, cost_times, balance_after, nonce, created_at
		   FROM entitlement_usage WHERE nonce = ?`, nonce).
		Scan(&u.ID, &u.CardID, &u.Code, &u.CostPoints, &u.CostTimes, &u.BalanceAfter, &u.Nonce, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Usage{}, ErrNotFound
	}
	return u, err
}

func (s *Store) InsertUsageTx(ctx context.Context, tx *sql.Tx, u Usage) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO entitlement_usage(card_id, code, cost_points, cost_times, balance_after, nonce, created_at)
		 VALUES(?,?,?,?,?,?,?)`,
		u.CardID, u.Code, u.CostPoints, u.CostTimes, u.BalanceAfter, u.Nonce, u.CreatedAt)
	if err != nil && isUniqueViolation(err) {
		return ErrConflict
	}
	return err
}

// DeductPointsTx 原子扣点：余额不足时不更新，返回 ok=false。
func (s *Store) DeductPointsTx(ctx context.Context, tx *sql.Tx, cardID, cost int64) (int64, bool, error) {
	res, err := tx.ExecContext(ctx,
		`UPDATE cards SET points = points - ? WHERE id = ? AND points >= ?`, cost, cardID, cost)
	if err != nil {
		return 0, false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return 0, false, err
	}
	var bal int64
	if err := tx.QueryRowContext(ctx, `SELECT points FROM cards WHERE id = ?`, cardID).Scan(&bal); err != nil {
		return 0, false, err
	}
	return bal, true, nil
}

// DeductTimesTx 原子扣次。
func (s *Store) DeductTimesTx(ctx context.Context, tx *sql.Tx, cardID, cost int64) (int64, bool, error) {
	res, err := tx.ExecContext(ctx,
		`UPDATE cards SET remain_times = remain_times - ? WHERE id = ? AND remain_times >= ?`, cost, cardID, cost)
	if err != nil {
		return 0, false, err
	}
	n, err := res.RowsAffected()
	if err != nil || n == 0 {
		return 0, false, err
	}
	var bal int64
	if err := tx.QueryRowContext(ctx, `SELECT remain_times FROM cards WHERE id = ?`, cardID).Scan(&bal); err != nil {
		return 0, false, err
	}
	return bal, true, nil
}

// SetCardPausedTx 更新计时卡暂停状态。
func (s *Store) SetCardPausedTx(ctx context.Context, tx *sql.Tx, cardID int64, paused bool) error {
	v := 0
	if paused {
		v = 1
	}
	_, err := tx.ExecContext(ctx, `UPDATE cards SET paused = ? WHERE id = ?`, v, cardID)
	return err
}
