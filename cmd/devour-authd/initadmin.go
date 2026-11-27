package main

import (
	"context"
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/config"
	"github.com/langyannb/devour-auth/internal/store"
)

const passwordAlphabet = "abcdefghijkmnopqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// runInitAdmin 建首个管理账号，随机密码与 TOTP 秘钥只打印一次（design.md 16.1）。
func runInitAdmin(cfg config.Config) error {
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

	n, err := st.AdminCount(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("已有 %d 个管理账号，拒绝覆盖（如需重置请手工处理 admins 表）", n)
	}

	pass, err := randomPassword(20)
	if err != nil {
		return err
	}
	passHash, err := auth.HashPassword(pass)
	if err != nil {
		return err
	}
	raw := make([]byte, 20)
	if _, err := rand.Read(raw); err != nil {
		return err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)

	if _, err := st.CreateAdmin(ctx, cfg.AdminUser, passHash, "admin", secret, 0, time.Now().Unix()); err != nil {
		return err
	}
	// 首个应用：表结构支持多个，当前只有 devour（design.md 第 3 节）。
	if _, err := st.EnsureApp(ctx, "devour", "Devour", time.Now().Unix()); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(tw, "用户名\t%s\n", cfg.AdminUser)
	fmt.Fprintf(tw, "密码\t%s\n", pass)
	fmt.Fprintf(tw, "TOTP 秘钥\t%s\n", secret)
	fmt.Fprintf(tw, "otpauth\totpauth://totp/Devour:%s?secret=%s&issuer=Devour\n", cfg.AdminUser, secret)
	tw.Flush()
	fmt.Fprintln(os.Stdout, "\n以上内容只显示这一次，保存后关闭终端。")
	return nil
}

func randomPassword(n int) (string, error) {
	out := make([]byte, n)
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	for i := range out {
		out[i] = passwordAlphabet[int(buf[i])%len(passwordAlphabet)]
	}
	return string(out), nil
}
