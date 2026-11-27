package httpapi

import (
	"net/http"
	"strings"
)

// hostGuard 按子域收紧可达路径：管理端、代理端、客户端各自只能打自己的接口。
// 子域隔离后，即便某端页面被注入脚本，也读不到另一端的响应（cookie 也按域隔离）。
func (s *Server) hostGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := stripPort(r.Host)
		path := r.URL.Path

		switch {
		case host == s.adminHost:
			if hasAnyPrefix(path, "/agent/", "/v1/", "/open/") {
				notFound(w)
				return
			}
		case host == s.agentHost:
			if hasAnyPrefix(path, "/admin/", "/v1/", "/open/") {
				notFound(w)
				return
			}
		case s.unbindHost != "" && host == s.unbindHost:
			// 用户自助解绑页：只开放页面与查卡/解绑接口
			if !(path == "/" || strings.HasPrefix(path, "/assets/") ||
				path == "/favicon.ico" || strings.HasPrefix(path, "/v1/query")) {
				notFound(w)
				return
			}
		case s.isNetHost(host):
			// 客户端域：只保留客户端协议与静态页
			if hasAnyPrefix(path, "/admin/", "/agent/") {
				notFound(w)
				return
			}
		}
		// 其他 Host（本地 127.0.0.1 等）不限制，方便调试
		next.ServeHTTP(w, r)
	})
}

// isNetHost 支持 DEVOUR_NET_HOST 用逗号列多个域（如裸域与 www）。
func (s *Server) isNetHost(host string) bool {
	if s.netHost == "" {
		return false
	}
	for _, h := range strings.Split(s.netHost, ",") {
		if strings.TrimSpace(h) == host {
			return true
		}
	}
	return false
}

func hasAnyPrefix(s string, prefixes ...string) bool {
	for _, p := range prefixes {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

func stripPort(host string) string {
	if i := strings.LastIndex(host, ":"); i > 0 {
		if strings.Count(host, ":") == 1 || strings.HasPrefix(host, "[") {
			return host[:i]
		}
	}
	return host
}

func notFound(w http.ResponseWriter) {
	writeErr(w, http.StatusNotFound, CodeBadInput, "路径不存在")
}
