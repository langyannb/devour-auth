package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/langyannb/devour-auth/internal/agent"
	"github.com/langyannb/devour-auth/internal/config"
	"github.com/langyannb/devour-auth/internal/store"
)

// runBills 汇总某月代理出卡流水生成账单；不带月份则取上月。
func runBills(cfg config.Config, month string) error {
	ctx := context.Background()
	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("找不到数据库 %s", cfg.DBPath)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	if month == "" {
		month = time.Now().AddDate(0, -1, 0).Format("2006-01")
	}
	n, err := agent.New(st).GenerateBills(ctx, month)
	if err != nil {
		return err
	}
	fmt.Printf("%s：生成 %d 张账单\n", month, n)
	return nil
}
