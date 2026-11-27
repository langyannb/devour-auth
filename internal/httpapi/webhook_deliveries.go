package httpapi

import "net/http"

// listWebhookDeliveries 最近 100 条投递记录，时间倒序。
func (s *Server) listWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListWebhookDeliveries(r.Context(), 100)
	if err != nil {
		internalErr(w, s.log, "查 webhook 投递记录失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		out = append(out, map[string]any{
			"id": d.ID, "webhook_id": d.WebhookID, "event": d.Event, "url": d.URL,
			"attempts": d.Attempts, "ok": d.OK, "status_code": d.StatusCode,
			"error": d.Error, "created_at": d.CreatedAt, "finished_at": d.FinishedAt,
		})
	}
	writeData(w, out)
}
