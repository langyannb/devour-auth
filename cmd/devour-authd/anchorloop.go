package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"log/slog"
	"time"

	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/store"
)

// startAnchoring 每天 00:05 对刚结束的一天做链尾签名（design.md 16.3）。
func startAnchoring(ctx context.Context, db *sql.DB, priv ed25519.PrivateKey, log *slog.Logger) {
	go func() {
		for {
			next := nextAnchorTime(time.Now())
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			day := next.Add(-5 * time.Minute).Format("2006-01-02")
			ok, err := audit.Anchor(ctx, db, priv, day, time.Now().Unix())
			switch {
			case err != nil:
				log.Error("审计锚定失败", "day", day, "err", err)
			case ok:
				log.Info("审计锚定完成", "day", day)
			}
		}
	}()
}

// startRollup 每天 00:05 汇总前一天的 daily_stats。
func startRollup(ctx context.Context, st *store.Store, log *slog.Logger) {
	go func() {
		for {
			next := nextAnchorTime(time.Now()).Add(2 * time.Minute)
			timer := time.NewTimer(time.Until(next))
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
			day := next.Add(-7 * time.Minute)
			start := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
			key := start.Format("2006-01-02")
			stat, err := st.RollupDay(ctx, key, start.Unix(), start.AddDate(0, 0, 1).Unix())
			if err != nil {
				log.Error("统计汇总失败", "day", key, "err", err)
				continue
			}
			log.Info("统计汇总完成", "day", key, "logins", stat.Logins, "activations", stat.Activations)
			// 回调投递记录留 30 天，够查问题就行
			if n, err := st.PruneDeliveries(ctx, time.Now().AddDate(0, 0, -30).Unix()); err != nil {
				log.Error("清理回调投递失败", "err", err)
			} else if n > 0 {
				log.Info("已清理回调投递", "rows", n)
			}
		}
	}()
}

// nextAnchorTime 下一个本地时间 00:05。
func nextAnchorTime(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 5, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
