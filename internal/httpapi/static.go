package httpapi

import (
	"bytes"
	"io/fs"
	"net/http"
	"strings"
	"time"
)

// spa 提供管理端静态文件，找不到的路径回退到 index.html（前端路由）。
func (s *Server) spa(w http.ResponseWriter, r *http.Request) {
	// API 前缀未匹配到具体路由时保持 JSON 错误，不要吐 HTML。
	if strings.HasPrefix(r.URL.Path, "/admin/api/") || strings.HasPrefix(r.URL.Path, "/v1/") ||
		strings.HasPrefix(r.URL.Path, "/agent/api/") || strings.HasPrefix(r.URL.Path, "/open/") {
		writeErr(w, http.StatusNotFound, CodeBadInput, "接口不存在")
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	data, err := fs.ReadFile(s.static, path)
	if err != nil {
		// SPA fallback：前端路由（/cards 等）刷新时也能返回入口页
		data, err = fs.ReadFile(s.static, "index.html")
		if err != nil {
			writeErr(w, http.StatusNotFound, CodeBadInput, "页面不存在")
			return
		}
		path = "index.html"
	}

	if strings.HasPrefix(path, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeContent(w, r, path, time.Time{}, bytes.NewReader(data))
}
