package main

import (
	"context"
	"fmt"
	"os"

	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/config"
	"github.com/langyannb/devour-auth/internal/store"
)

// runVerifyAudit 重算整条审计链，断链时打印断点（design.md 16.3）。
func runVerifyAudit(cfg config.Config) error {
	ctx := context.Background()
	path := cfg.DBPath
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("找不到数据库 %s", path)
	}
	st, err := store.Open(ctx, path)
	if err != nil {
		return err
	}
	defer st.Close()

	n, err := audit.Verify(ctx, st.DB())
	if err != nil {
		return fmt.Errorf("审计链断裂（已校验 %d 条）：%w", n, err)
	}
	fmt.Printf("审计链完整：%d 条记录\n", n)
	return nil
}
