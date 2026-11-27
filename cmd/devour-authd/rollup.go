package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/langyannb/devour-auth/internal/config"
	"github.com/langyannb/devour-auth/internal/store"
)

// runRollup 手动汇总某天（默认昨天）的 daily_stats。
func runRollup(cfg config.Config, day string) error {
	ctx := context.Background()
	if _, err := os.Stat(cfg.DBPath); err != nil {
		return fmt.Errorf("找不到数据库 %s", cfg.DBPath)
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()

	if day == "" {
		day = time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	}
	t, err := time.ParseInLocation("2006-01-02", day, time.Local)
	if err != nil {
		return fmt.Errorf("日期格式应为 YYYY-MM-DD")
	}
	stat, err := st.RollupDay(ctx, day, t.Unix(), t.AddDate(0, 0, 1).Unix())
	if err != nil {
		return err
	}
	fmt.Printf("%s：登录 %d 次、激活 %d 张、独立设备 %d 台、出卡金额 %d 分\n",
		day, stat.Logins, stat.Activations, stat.UniqueDevices, stat.IssuedCents)
	return nil
}
