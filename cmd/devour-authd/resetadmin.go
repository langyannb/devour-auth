package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/config"
	"github.com/langyannb/devour-auth/internal/store"
)

// runResetAdmin 改管理员用户名与密码（密码从 stdin 读，避免留在 shell 历史里）。
func runResetAdmin(cfg config.Config, username string) error {
	if username == "" {
		return fmt.Errorf("用法：reset-admin <新用户名>（密码从 stdin 读）")
	}
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return fmt.Errorf("没有从 stdin 读到密码")
	}
	password := strings.TrimSpace(sc.Text())
	if len(password) < auth.MinPasswordLen {
		return fmt.Errorf("密码至少 %d 位", auth.MinPasswordLen)
	}

	ctx := context.Background()
	if dir := filepath.Dir(cfg.DBPath); dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	st, err := store.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.Migrate(ctx); err != nil {
		return err
	}

	admins, err := st.ListAdminUsernames(ctx)
	if err != nil {
		return err
	}
	if len(admins) == 0 {
		return fmt.Errorf("还没有管理员，先跑 init-admin")
	}
	target := admins[0]
	if len(admins) > 1 {
		return fmt.Errorf("有多个管理员（%s），请指定要改哪一个", strings.Join(admins, ", "))
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}
	now := time.Now().Unix()

	// 改用户名 + 密码，再踢掉旧会话，最后落审计
	a, err := st.AdminByUsername(ctx, target)
	if err != nil {
		return err
	}
	if err := st.SetAdminCredentials(ctx, a.ID, username, hash); err != nil {
		if err == store.ErrConflict {
			return fmt.Errorf("用户名 %s 已存在", username)
		}
		return err
	}
	n, err := st.RevokeAllAdminSessions(ctx, a.ID, now)
	if err != nil {
		return err
	}

	tx, err := st.DB().BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     now,
		Actor:  "system:reset-admin",
		Action: "admin.reset_credentials",
		Target: fmt.Sprintf("admin:%d", a.ID),
		Before: fmt.Sprintf(`{"username":%q}`, a.Username),
		After:  fmt.Sprintf(`{"username":%q}`, username),
	}); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	fmt.Printf("已更新：%s → %s，吊销旧会话 %d 个\n", a.Username, username, n)
	return nil
}
