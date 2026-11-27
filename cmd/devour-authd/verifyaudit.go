package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"os"
	"time"

	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/config"
	"github.com/langyannb/devour-auth/internal/store"
)

// runVerifyAudit 重算整条审计链并校验每日锚点（design.md 16.3）。
func runVerifyAudit(cfg config.Config) error {
	ctx := context.Background()
	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("找不到数据库 %s", cfg.DBPath)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	n, err := audit.Verify(ctx, st.DB())
	if err != nil {
		return fmt.Errorf("审计链断裂（已校验 %d 条）：%w", n, err)
	}

	priv, keyErr := audit.LoadKey(cfg.AuditKeyPath)
	if keyErr != nil {
		fmt.Printf("审计链完整：%d 条（未找到私钥 %s，跳过锚点校验）\n", n, cfg.AuditKeyPath)
		return nil
	}
	m, err := audit.VerifyAnchors(ctx, st.DB(), priv.Public().(ed25519.PublicKey))
	if err != nil {
		return fmt.Errorf("锚点校验失败：%w", err)
	}
	fmt.Printf("审计链完整：%d 条，锚点 %d 个\n", n, m)
	return nil
}

// runAnchor 手动对昨天补一次锚定，供 cron 或运维使用。
func runAnchor(cfg config.Config) error {
	ctx := context.Background()
	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("找不到数据库 %s", cfg.DBPath)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	priv, err := audit.LoadOrCreateKey(cfg.AuditKeyPath)
	if err != nil {
		return err
	}
	day := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	ok, err := audit.Anchor(ctx, st.DB(), priv, day, time.Now().Unix())
	if err != nil {
		return err
	}
	if !ok {
		fmt.Printf("锚点已存在：%s\n", day)
		return nil
	}
	fmt.Printf("已锚定：%s\n", day)
	return nil
}
