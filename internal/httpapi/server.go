package httpapi

import (
	"database/sql"
	"log/slog"
	"net/http"
	"time"
)

type Server struct {
	db  *sql.DB
	log *slog.Logger
}

func New(db *sql.DB, log *slog.Logger) *Server {
	return &Server{db: db, log: log}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.healthz)
	// 未匹配路径也走统一错误体（JSON 端点，不返回 HTML）。
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, CodeBadInput, "接口不存在")
	})
	return s.withLogging(mux)
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.db.PingContext(ctx); err != nil {
		s.log.Error("healthz: 数据库不可用", "err", err)
		writeErr(w, http.StatusServiceUnavailable, CodeInternal, "数据库不可用")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"server_time": time.Now().Unix(),
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
