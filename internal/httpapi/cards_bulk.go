package httpapi

import (
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/audit"
)

func scopeName(scope string) string {
	switch scope {
	case "active":
		return "已激活"
	case "unused":
		return "未使用"
	case "expired":
		return "已过期"
	case "all":
		return "全部"
	}
	return scope
}

// bulkPreview 批量操作前先看会影响多少张。
func (s *Server) bulkPreview(w http.ResponseWriter, r *http.Request) {
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	scope := r.URL.Query().Get("scope")
	total, timed, err := s.st.CountCardsByScope(r.Context(), app.ID, scope)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "范围不合法")
		return
	}
	writeData(w, map[string]any{"scope": scope, "total": total, "time_cards": timed})
}

// bulkCards 按范围批量加时/扣时，或清理已过期卡。写一条汇总审计。
func (s *Server) bulkCards(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope   string `json:"scope"`
		Action  string `json:"action"` // add | subtract | recycle
		Seconds int64  `json:"seconds"`
	}
	if !decode(w, r, &req) {
		return
	}
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	if req.Action != "recycle" {
		if req.Seconds <= 0 || req.Seconds > 10*365*86400 {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "时长不合法")
			return
		}
	}
	ctx := r.Context()
	now := s.now().Unix()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		internalErr(w, s.log, "批量操作失败", err)
		return
	}
	defer tx.Rollback()

	var affected int64
	switch req.Action {
	case "add":
		affected, err = s.st.BulkAddExpire(ctx, tx, app.ID, req.Scope, req.Seconds)
	case "subtract":
		affected, err = s.st.BulkSubExpire(ctx, tx, app.ID, req.Scope, req.Seconds, now)
	case "recycle":
		affected, err = s.st.BulkRecycleExpired(ctx, tx, app.ID)
	default:
		writeErr(w, http.StatusBadRequest, CodeBadInput, "动作不合法")
		return
	}
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "范围不合法")
		return
	}
	c := adminFrom(r)
	after := fmt.Sprintf(`{"scope":%q,"action":%q,"seconds":%d,"affected":%d}`,
		req.Scope, req.Action, req.Seconds, affected)
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     now,
		Actor:  fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
		Action: "card.bulk_" + req.Action,
		Target: fmt.Sprintf("app:%d", app.ID),
		After:  after,
		IP:     clientIP(r),
	}); err != nil {
		internalErr(w, s.log, "批量操作失败", err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalErr(w, s.log, "批量操作失败", err)
		return
	}
	writeData(w, map[string]any{
		"scope": req.Scope, "scope_name": scopeName(req.Scope),
		"action": req.Action, "seconds": req.Seconds, "affected": affected,
	})
}
