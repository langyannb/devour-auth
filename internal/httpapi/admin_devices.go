package httpapi

import (
	"fmt"
	"net/http"

	"github.com/langyannb/devour-auth/internal/store"
)

func deviceJSON(d store.Device) map[string]any {
	var unbound any
	if d.UnboundAt.Valid {
		unbound = d.UnboundAt.Int64
	}
	return map[string]any{
		"id": d.ID, "card_id": d.CardID, "fingerprint": d.Fingerprint,
		"model": d.Model, "brand": d.Brand, "os": d.OS, "app_version": d.AppVersion,
		"first_seen": d.FirstSeen, "last_seen": d.LastSeen,
		"banned": d.Banned == 1, "unbound_at": unbound,
	}
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	list, total, err := s.st.ListDevices(r.Context(),
		queryInt64(r, "card_id"), effectiveAppID(r),
		r.URL.Query().Get("fingerprint"),
		r.URL.Query().Get("all") == "1",
		int(queryInt64(r, "limit")), int(queryInt64(r, "offset")))
	if err != nil {
		internalErr(w, s.log, "查设备失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		out = append(out, deviceJSON(d))
	}
	writeData(w, map[string]any{"items": out, "total": total})
}

// deviceAction 设备解绑 / 封禁 / 解封。
func (s *Server) deviceAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(r)
		if !ok {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
			return
		}

		if !s.deviceAllowed(w, r, id) {
			return
		}
		ctx := r.Context()
		now := s.now().Unix()

		var err error
		switch action {
		case "unbind":
			_, err = s.st.UnbindDeviceByID(ctx, id, now)
		case "ban":
			err = s.st.SetDeviceBanned(ctx, id, true)
		case "unban":
			err = s.st.SetDeviceBanned(ctx, id, false)
		}
		if err != nil {
			internalErr(w, s.log, "设备操作失败", err)
			return
		}
		c := adminFrom(r)
		_ = s.writeAudit(ctx, c, "device."+action, fmt.Sprintf("device:%d", id), "",
			`{"action":"`+action+`"}`, clientIP(r))
		writeData(w, map[string]any{"id": id, "action": action, "ok": true})
	}
}

func sessionJSON(sr store.SessionRow) map[string]any {
	return map[string]any{
		"id": sr.ID, "card_id": sr.CardID, "kami": sr.Kami,
		"device_id": sr.DeviceID, "fingerprint": sr.Fingerprint,
		"access_exp": sr.AccessExp, "refresh_exp": sr.RefreshExp,
		"last_heartbeat": sr.LastHeartbeat, "ip": sr.IP, "ua": sr.UA, "created_at": sr.CreatedAt,
	}
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request) {
	list, total, err := s.st.ListSessions(r.Context(), effectiveAppID(r), s.now().Unix(),
		int(queryInt64(r, "limit")), int(queryInt64(r, "offset")))
	if err != nil {
		internalErr(w, s.log, "查在线会话失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, sr := range list {
		out = append(out, sessionJSON(sr))
	}
	writeData(w, map[string]any{"items": out, "total": total})
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}

	if !s.sessionAllowed(w, r, id) {
		return
	}
	ctx := r.Context()
	if err := s.st.RevokeSessionByID(ctx, id, s.now().Unix()); err != nil {
		internalErr(w, s.log, "踢下线失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "session.revoke", fmt.Sprintf("session:%d", id), "", `{"revoked":true}`, clientIP(r))
	writeData(w, map[string]any{"id": id, "revoked": true})
}

func (s *Server) listCardDevices(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}

	if !s.cardAllowed(w, r, id) {
		return
	}
	list, _, err := s.st.ListDevices(r.Context(), id, 0, "", true, 100, 0)
	if err != nil {
		internalErr(w, s.log, "查设备失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, d := range list {
		out = append(out, deviceJSON(d))
	}
	writeData(w, out)
}

func (s *Server) listCardSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}

	if !s.cardAllowed(w, r, id) {
		return
	}
	list, _, err := s.st.ListSessionsByCard(r.Context(), id, 50, 0)
	if err != nil {
		internalErr(w, s.log, "查会话失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, sr := range list {
		out = append(out, sessionJSON(sr))
	}
	writeData(w, out)
}
