package httpapi

import (
	"net/http"
)

// integrationInfo 对接信息：当前应用、当前账号、以及这个应用的开放接口密钥（含 secret）。
// 绑定应用的账号只能看到自己的那份；全局管理员看当前选中的应用。
func (s *Server) integrationInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "应用不存在")
		return
	}
	c := adminFrom(r)
	keys, err := s.st.ListAPIKeys(ctx, app.ID)
	if err != nil {
		internalErr(w, s.log, "查密钥失败", err)
		return
	}
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, map[string]any{
			"id": k.ID, "name": k.Name, "key_id": k.KeyID, "secret": k.Secret,
			"scopes": k.Scopes, "status": k.Status, "created_at": k.CreatedAt,
		})
	}
	writeData(w, map[string]any{
		"app":      map[string]any{"id": app.ID, "code": app.Code, "name": app.Name},
		"admin":    map[string]any{"username": c.Admin.Username},
		"api_keys": out,
	})
}
