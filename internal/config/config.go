// Package config 从环境变量读取启动配置，只在 main 中调用（AGENTS.md 2.3）。
package config

import "os"

type Config struct {
	ListenAddr   string // DEVOUR_LISTEN
	DBPath       string // DEVOUR_DB
	Env          string // DEVOUR_ENV: dev | prod
	AdminUser    string // DEVOUR_ADMIN_USER，仅 init-admin 使用
	AuditKeyPath string // DEVOUR_AUDIT_KEY，审计锚定私钥
	AdminHost    string // DEVOUR_ADMIN_HOST，管理端子域
	AgentHost    string // DEVOUR_AGENT_HOST，代理端子域
	NetHost      string // DEVOUR_NET_HOST，客户端域
	UnbindHost   string // DEVOUR_UNBIND_HOST，用户自助解绑页
	BackupDir    string // DEVOUR_BACKUP_DIR，备份目录（安全体检用）
}

func Load() Config {
	return Config{
		ListenAddr:   envOr("DEVOUR_LISTEN", "127.0.0.1:6760"),
		DBPath:       envOr("DEVOUR_DB", "data/app.db"),
		Env:          envOr("DEVOUR_ENV", "dev"),
		AdminUser:    envOr("DEVOUR_ADMIN_USER", "admin"),
		AuditKeyPath: envOr("DEVOUR_AUDIT_KEY", "data/audit_ed25519.key"),
		AdminHost:    envOr("DEVOUR_ADMIN_HOST", "admin.namikao.com"),
		AgentHost:    envOr("DEVOUR_AGENT_HOST", "agent.namikao.com"),
		NetHost:      envOr("DEVOUR_NET_HOST", "net.namikao.com"),
		UnbindHost:   envOr("DEVOUR_UNBIND_HOST", "unbind.namikao.com"),
		BackupDir:    envOr("DEVOUR_BACKUP_DIR", "/var/backups/devour"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
