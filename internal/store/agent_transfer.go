package store

import (
	"context"
	"fmt"
)

// TransferAgentQuota 上级把额度划给下级：扣上级、加下级，各记一条流水。
// 上级额度 -1（不限）时不扣上限，但下级照加。
func (s *Store) TransferAgentQuota(ctx context.Context, fromID, toID, delta int64, note string, now int64) error {
	if delta <= 0 {
		return fmt.Errorf("划转额度必须大于 0")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var fromQuota int64
	if err := tx.QueryRowContext(ctx, `SELECT quota_cents FROM agents WHERE id = ?`, fromID).Scan(&fromQuota); err != nil {
		return err
	}
	if fromQuota >= 0 {
		if fromQuota < delta {
			return fmt.Errorf("余额不足：还剩 %d 分", fromQuota)
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE agents SET quota_cents = quota_cents - ? WHERE id = ?`, delta, fromID); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE agents SET quota_cents = quota_cents + ? WHERE id = ?`, delta, toID); err != nil {
		return err
	}
	if err := s.AddAgentLedgerTx(ctx, tx, fromID, "adjust", -delta, "转给下级 "+note, now); err != nil {
		return err
	}
	if err := s.AddAgentLedgerTx(ctx, tx, toID, "adjust", delta, "上级划入 "+note, now); err != nil {
		return err
	}
	return tx.Commit()
}
