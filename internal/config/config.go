// Package config 从环境变量读取启动配置，只在 main 中调用（AGENTS.md 2.3）。
package config

import "os"

type Config struct {
	ListenAddr string // DEVOUR_LISTEN
	DBPath     string // DEVOUR_DB
	Env        string // DEVOUR_ENV: dev | prod
	AdminUser  string // DEVOUR_ADMIN_USER，仅 init-admin 使用
}

func Load() Config {
	return Config{
		ListenAddr: envOr("DEVOUR_LISTEN", "127.0.0.1:6760"),
		DBPath:     envOr("DEVOUR_DB", "data/app.db"),
		Env:        envOr("DEVOUR_ENV", "dev"),
		AdminUser:  envOr("DEVOUR_ADMIN_USER", "admin"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
