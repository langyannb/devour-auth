package httpapi

import (
	"crypto/ed25519"
	"net/http"
	"os"
	"path/filepath"
	"sort"

	"github.com/langyannb/devour-auth/internal/audit"
)

// healthCheck 安全体检：审计链、锚点、备份、密钥、库大小。
func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	out := map[string]any{}

	if n, err := audit.Verify(ctx, s.st.DB()); err != nil {
		out["audit_chain_ok"] = false
		out["audit_entries"] = n
		out["audit_error"] = err.Error()
	} else {
		out["audit_chain_ok"] = true
		out["audit_entries"] = n
	}

	if m, err := audit.VerifyAnchors(ctx, s.st.DB(), s.ticketKey.Public().(ed25519.PublicKey)); err == nil {
		out["anchors"] = m
	} else {
		out["anchors"] = -1
		out["anchors_error"] = err.Error()
	}

	var lastDay string
	var lastID int64
	if err := s.st.DB().QueryRowContext(ctx,
		`SELECT day, last_id FROM audit_anchor ORDER BY day DESC LIMIT 1`).Scan(&lastDay, &lastID); err == nil {
		out["last_anchor_day"] = lastDay
		out["last_anchor_id"] = lastID
	}

	// 备份文件
	files, _ := filepath.Glob(filepath.Join(s.backupDir, "*.gpg"))
	sort.Strings(files)
	out["backups"] = len(files)
	if len(files) > 0 {
		latest := files[len(files)-1]
		out["last_backup_file"] = filepath.Base(latest)
		if st, err := os.Stat(latest); err == nil {
			out["last_backup_at"] = st.ModTime().Unix()
			out["last_backup_size"] = st.Size()
		}
	}

	// 数据库与密钥文件
	if st, err := os.Stat(s.dbPath); err == nil {
		out["db_size"] = st.Size()
	}
	out["server_time"] = s.now().Unix()
	out["admin_host"] = s.adminHost
	out["agent_host"] = s.agentHost
	writeData(w, out)
}
