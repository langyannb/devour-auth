package httpapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

var appCodeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{1,31}$`)

func (s *Server) createApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Code string `json:"code"`
		Name string `json:"name"`
	}
	if !decode(w, r, &req) {
		return
	}
	code := strings.TrimSpace(strings.ToLower(req.Code))
	name := strings.TrimSpace(req.Name)
	if !appCodeRe.MatchString(code) {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "code 需为 2-32 位小写字母/数字/下划线/短横线")
		return
	}
	if name == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "名称必填")
		return
	}
	id, err := s.st.CreateApp(ctx, code, name, s.now().Unix())
	if err != nil {
		writeErr(w, http.StatusConflict, CodeBadInput, "应用 code 已存在")
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "app.create", fmt.Sprintf("app:%d", id), "",
		fmt.Sprintf(`{"code":%q,"name":%q}`, code, name), clientIP(r))
	writeData(w, map[string]any{"id": id, "code": code, "name": name, "active": 1})
}

func (s *Server) updateApp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	old, err := s.st.AppByID(ctx, id)
	if err != nil || (adminScopeID(r) > 0 && id != adminScopeID(r)) {
		writeErr(w, http.StatusNotFound, CodeBadInput, "应用不存在")
		return
	}
	var req struct {
		Name   *string `json:"name"`
		Active *int64  `json:"active"`
	}
	if !decode(w, r, &req) {
		return
	}
	name, active := old.Name, old.Active
	if req.Name != nil && strings.TrimSpace(*req.Name) != "" {
		name = strings.TrimSpace(*req.Name)
	}
	if req.Active != nil {
		if *req.Active != 0 && *req.Active != 1 {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "active 只能是 0 或 1")
			return
		}
		active = *req.Active
	}
	if err := s.st.UpdateApp(ctx, id, name, active); err != nil {
		internalErr(w, s.log, "更新应用失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "app.update", fmt.Sprintf("app:%d", id),
		fmt.Sprintf(`{"name":%q,"active":%d}`, old.Name, old.Active),
		fmt.Sprintf(`{"name":%q,"active":%d}`, name, active), clientIP(r))
	writeData(w, map[string]any{"id": id, "code": old.Code, "name": name, "active": active})
}
