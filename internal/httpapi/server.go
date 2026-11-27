package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

type Server struct {
	st  *store.Store
	log *slog.Logger
	env string
	lim *auth.Limiter
	now func() time.Time
}

func New(st *store.Store, log *slog.Logger, env string) *Server {
	return &Server{
		st:  st,
		log: log,
		env: env,
		lim: auth.NewLimiter(),
		now: time.Now,
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	mux.HandleFunc("POST /admin/api/login", s.adminLogin)
	mux.HandleFunc("POST /admin/api/logout", s.requireAdmin(s.adminLogout))
	mux.HandleFunc("GET /admin/api/me", s.requireAdmin(s.adminMe))
	mux.HandleFunc("POST /admin/api/reauth", s.requireAdmin(s.adminReauth))
	// 未匹配路径也走统一错误体（JSON 端点，不返回 HTML）。
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, CodeBadInput, "接口不存在")
	})
	return s.withLogging(mux)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.st.DB().PingContext(r.Context()); err != nil {
		s.log.Error("healthz: 数据库不可用", "err", err)
		writeErr(w, http.StatusServiceUnavailable, CodeInternal, "数据库不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"server_time": s.now().Unix(),
	})
}

func (s *Server) withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Debug("http", "method", r.Method, "path", r.URL.Path,
			"dur_ms", time.Since(start).Milliseconds())
	})
}
