package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"log/slog"
	"time"

	"github.com/langyannb/devour-auth/internal/audit"
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

// nextAnchorTime 下一个本地时间 00:05。
func nextAnchorTime(now time.Time) time.Time {
	next := time.Date(now.Year(), now.Month(), now.Day(), 0, 5, 0, 0, now.Location())
	if !next.After(now) {
		next = next.AddDate(0, 0, 1)
	}
	return next
}
