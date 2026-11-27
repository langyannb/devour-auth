package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/store"
)

func (s *Server) listWebhooks(w http.ResponseWriter, r *http.Request) {
	app, _ := s.appFromQuery(r)
	list, err := s.st.ListWebhooks(r.Context(), app.ID)
	if err != nil {
		internalErr(w, s.log, "查 webhook 失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, h := range list {
		out = append(out, map[string]any{
			"id": h.ID, "url": h.URL, "events": h.Events, "active": h.Active, "created_at": h.CreatedAt,
		})
	}
	writeData(w, out)
}

func (s *Server) createWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, _ := s.appFromQuery(r)
	var req struct {
		URL    string `json:"url"`
		Events string `json:"events"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.URL == "" || req.Events == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "url 与 events 必填")
		return
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		internalErr(w, s.log, "生成 secret 失败", err)
		return
	}
	secret := hex.EncodeToString(raw)
	id, err := s.st.CreateWebhook(ctx, store.Webhook{
		AppID: app.ID, URL: req.URL, Secret: secret, Events: req.Events,
		Active: 1, CreatedAt: s.now().Unix(),
	})
	if err != nil {
		internalErr(w, s.log, "创建 webhook 失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "webhook.create", fmt.Sprintf("webhook:%d", id), "",
		fmt.Sprintf(`{"url":%q,"events":%q}`, req.URL, req.Events), clientIP(r))
	// secret 只在创建时返回一次，用于对方校验 X-Devour-Signature
	writeData(w, map[string]any{"id": id, "url": req.URL, "events": req.Events, "secret": secret})
}

func (s *Server) toggleWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req struct {
		Active bool `json:"active"`
	}
	if !decode(w, r, &req) {
		return
	}
	if err := s.st.SetWebhookActive(r.Context(), id, req.Active); err != nil {
		internalErr(w, s.log, "更新 webhook 失败", err)
		return
	}
	writeData(w, map[string]any{"id": id, "active": req.Active})
}

func (s *Server) deleteWebhook(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	if err := s.st.DeleteWebhook(r.Context(), id); err != nil {
		internalErr(w, s.log, "删除 webhook 失败", err)
		return
	}
	writeData(w, map[string]any{"id": id, "deleted": true})
}
