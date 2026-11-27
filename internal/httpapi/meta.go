package httpapi

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/apierr"
	"github.com/langyannb/devour-auth/internal/store"
)

// adminScopeID 会话可操作的应用：绑定应用的账号只能用自己的，0 = 全局。
func adminScopeID(r *http.Request) int64 {
	if c, ok := r.Context().Value(adminCtxKey{}).(adminCtx); ok && c.Admin.AppID > 0 {
		return c.Admin.AppID
	}
	return 0
}

// effectiveAppID 当前请求实际生效的应用：绑定账号强制自己的，其余按 ?app_id=（0 = 全部）。
func effectiveAppID(r *http.Request) int64 {
	if id := adminScopeID(r); id > 0 {
		return id
	}
	return queryInt64(r, "app_id")
}

// 以下四个 helper 防跨应用越权：受限账号按 id 操作别的应用的对象一律 404。

func (s *Server) cardAllowed(w http.ResponseWriter, r *http.Request, cardID int64) bool {
	scope := adminScopeID(r)
	if scope == 0 {
		return true
	}
	c, err := s.st.CardByID(r.Context(), cardID)
	if err != nil || c.AppID != scope {
		writeErr(w, http.StatusNotFound, CodeBadInput, "卡不存在")
		return false
	}
	return true
}

func (s *Server) agentAllowed(w http.ResponseWriter, r *http.Request, agentID int64) bool {
	scope := adminScopeID(r)
	if scope == 0 {
		return true
	}
	a, err := s.st.AgentByID(r.Context(), agentID)
	if err != nil || a.AppID != scope {
		writeErr(w, http.StatusNotFound, CodeBadInput, "代理不存在")
		return false
	}
	return true
}

func (s *Server) deviceAllowed(w http.ResponseWriter, r *http.Request, deviceID int64) bool {
	scope := adminScopeID(r)
	if scope == 0 {
		return true
	}
	d, err := s.st.DeviceByID(r.Context(), deviceID)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "设备不存在")
		return false
	}
	return s.cardAllowed(w, r, d.CardID)
}

func (s *Server) sessionAllowed(w http.ResponseWriter, r *http.Request, sessionID int64) bool {
	scope := adminScopeID(r)
	if scope == 0 {
		return true
	}
	app, err := s.st.SessionCardApp(r.Context(), sessionID)
	if err != nil || app != scope {
		writeErr(w, http.StatusNotFound, CodeBadInput, "会话不存在")
		return false
	}
	return true
}

// appFromQuery 解析当前应用：优先 ?app_id=，其次 ?app=code，都没有则取第一个。
// 绑定应用的账号忽略入参，直接用自己的应用。
func (s *Server) appFromQuery(r *http.Request) (store.App, error) {
	if id := adminScopeID(r); id > 0 {
		return s.st.AppByID(r.Context(), id)
	}
	if id := queryInt64(r, "app_id"); id > 0 {
		return s.st.AppByID(r.Context(), id)
	}
	if code := r.URL.Query().Get("app"); code != "" {
		return s.st.AppByCode(r.Context(), code)
	}
	apps, err := s.st.ListApps(r.Context())
	if err != nil || len(apps) == 0 {
		return store.App{}, store.ErrNotFound
	}
	return apps[0], nil
}

// rolloutHit 安装量灰度：把设备指纹哈希到 0..9999，小于 rollout_bp 才下发。
func rolloutHit(fingerprint, version string, rolloutBP int64) bool {
	if rolloutBP >= 10000 {
		return true
	}
	if fingerprint == "" || rolloutBP <= 0 {
		return false
	}
	sum := sha256.Sum256([]byte(fingerprint + "|" + version))
	return int64(binary.BigEndian.Uint32(sum[:4])%10000) < rolloutBP
}

// meta 免登录：公告、最新版本、远程变量、维护开关（design.md 第 12 节）。
func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := s.appFromQuery(r)
	if err != nil {
		writeErr(w, http.StatusNotFound, apierr.BadInput, "应用不存在")
		return
	}

	flags, err := s.st.ListFlags(ctx, app.ID)
	if err != nil {
		internalErr(w, s.log, "查变量失败", err)
		return
	}
	notices, err := s.st.ListNotices(ctx, app.ID, true)
	if err != nil {
		internalErr(w, s.log, "查公告失败", err)
		return
	}

	// card_mode=free：客户端不校验卡密自行放行；服务端不做特殊处理
	mode := "paid"
	if flags["card_mode"] == "free" {
		mode = "free"
	}
	out := map[string]any{
		"app":           app.Code,
		"server_time":   s.now().Unix(),
		"mode":          mode,
		"maintenance":   flags["maintenance"] == "1",
		"notices":       noticesJSON(notices),
		"flags":         flags,
		"ticket_pubkey": base64.StdEncoding.EncodeToString(s.ticketKey.Public().(ed25519.PublicKey)),
	}

	// 版本：带 ?fp= 时按灰度比例决定是否下发
	if rel, err := s.st.LatestRelease(ctx, app.ID); err == nil {
		if rolloutHit(r.URL.Query().Get("fp"), rel.Version, rel.RolloutBP) {
			out["release"] = map[string]any{
				"version": rel.Version, "info": rel.Info, "url": rel.URL,
				"must": rel.Must == 1, "rollout_bp": rel.RolloutBP,
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": out})
}

func noticesJSON(list []store.Notice) []map[string]any {
	out := make([]map[string]any, 0, len(list))
	for _, n := range list {
		out = append(out, map[string]any{
			"id": n.ID, "kind": n.Kind, "title": n.Title, "body": n.Body,
		})
	}
	return out
}

// offlineTicket 服务端签发的离线凭证，客户端断网时本地验签（本期只做签发）。
type offlineTicket struct {
	Card         string   `json:"card"`
	Device       string   `json:"device"`
	Exp          int64    `json:"exp"`
	Entitlements []string `json:"entitlements"`
	Points       int64    `json:"points"`
}

func (s *Server) offlineTicket(w http.ResponseWriter, r *http.Request) {
	tok := bearer(r)
	if tok == "" {
		writeErr(w, http.StatusUnauthorized, apierr.Unauthorized, "缺少访问令牌")
		return
	}
	c, snap, fingerprint, ents, err := s.clients.TicketData(r.Context(), tok)
	if err != nil {
		s.writeAPIError(w, err)
		return
	}

	now := s.now()
	maxExp := now.Add(24 * time.Hour).Unix()
	if snap.OfflineHours > 0 {
		maxExp = now.Add(time.Duration(snap.OfflineHours) * time.Hour).Unix()
	}
	if c.ExpireAt > 0 && c.ExpireAt < maxExp {
		maxExp = c.ExpireAt
	}

	codes := make([]string, 0, len(ents))
	for _, e := range ents {
		codes = append(codes, e.Code)
	}
	ticket := offlineTicket{
		Card:         c.Kami,
		Device:       fingerprint,
		Exp:          maxExp,
		Entitlements: codes,
		Points:       c.Points,
	}
	body, err := json.Marshal(ticket)
	if err != nil {
		internalErr(w, s.log, "序列化票据失败", err)
		return
	}
	sig := ed25519.Sign(s.ticketKey, body)

	writeData(w, map[string]any{
		"ticket":      base64.StdEncoding.EncodeToString(body),
		"signature":   base64.StdEncoding.EncodeToString(sig),
		"exp":         maxExp,
		"public_key":  base64.StdEncoding.EncodeToString(s.ticketKey.Public().(ed25519.PublicKey)),
		"server_time": now.Unix(),
	})
}
