package httpapi

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/langyannb/devour-auth/internal/apierr"
	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

const (
	openMaxSkew    = 300
	openNonceTTL   = 5 * time.Minute
	openMaxBodyLen = 1 << 20
)

type openKeyCtxKey struct{}

func openKeyFrom(r *http.Request) store.APIKey {
	return r.Context().Value(openKeyCtxKey{}).(store.APIKey)
}

func hasScope(scopes, want string) bool {
	for _, s := range strings.Split(scopes, ",") {
		if strings.TrimSpace(s) == want {
			return true
		}
	}
	return false
}

// sign 与文档一致：hex(hmac_sha256(secret, method \n path \n ts \n nonce \n body))
func sign(secret, method, path, ts, nonce string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(method + "\n" + path + "\n" + ts + "\n" + nonce + "\n"))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// requireAPIKey 校验开放接口签名（design.md 第 13 节）。
func (s *Server) requireAPIKey(scope string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		keyID := r.Header.Get("X-Api-Key")
		tsStr := r.Header.Get("X-Timestamp")
		nonce := r.Header.Get("X-Nonce")
		sig := strings.ToLower(r.Header.Get("X-Signature"))
		if keyID == "" || tsStr == "" || nonce == "" || sig == "" {
			writeErr(w, http.StatusUnauthorized, apierr.Unauthorized, "缺少签名头")
			return
		}

		now := s.now().Unix()
		ts, err := strconv.ParseInt(tsStr, 10, 64)
		if err != nil || ts-now > openMaxSkew || now-ts > openMaxSkew {
			writeErr(w, http.StatusUnauthorized, apierr.Unauthorized, "时间偏差超过 5 分钟")
			return
		}

		k, err := s.st.APIKeyByKeyID(r.Context(), keyID)
		if err != nil || k.Status != "active" {
			writeErr(w, http.StatusUnauthorized, apierr.Unauthorized, "密钥无效")
			return
		}
		if !hasScope(k.Scopes, scope) {
			writeErr(w, http.StatusForbidden, apierr.Unauthorized, "缺少权限 "+scope)
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, openMaxBodyLen+1))
		if err != nil || len(body) > openMaxBodyLen {
			writeErr(w, http.StatusBadRequest, apierr.BadInput, "请求体过大")
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))

		want := sign(k.Secret, r.Method, r.URL.Path, tsStr, nonce, body)
		if subtle.ConstantTimeCompare([]byte(want), []byte(sig)) != 1 {
			writeErr(w, http.StatusUnauthorized, apierr.Unauthorized, "签名错误")
			return
		}

		// nonce 5 分钟内只允许用一次
		if !s.openNonces.Allow("open:"+keyID+":"+nonce, 1, openNonceTTL) {
			writeErr(w, http.StatusUnauthorized, apierr.Unauthorized, "nonce 重复")
			return
		}

		_ = s.st.TouchAPIKey(r.Context(), k.ID, now)
		ctx := context.WithValue(r.Context(), openKeyCtxKey{}, k)
		next(w, r.WithContext(ctx))
	}
}

// ---------- 业务端点 ----------

func (s *Server) openGenerate(w http.ResponseWriter, r *http.Request) {
	var req struct {
		PolicyID int64  `json:"policy_id"`
		Qty      int    `json:"qty"`
		Prefix   string `json:"prefix"`
		Note     string `json:"note"`
	}
	if !decode(w, r, &req) {
		return
	}
	ctx := r.Context()
	p, err := s.st.PolicyByID(ctx, req.PolicyID)
	if err != nil {
		writeErr(w, http.StatusNotFound, apierr.BadInput, "卡类型不存在")
		return
	}
	k := openKeyFrom(r)
	res, err := s.cards.Generate(ctx, card.GenerateReq{
		AppID: p.AppID, PolicyID: p.ID, Qty: req.Qty, Prefix: req.Prefix,
		CreatedBy: 0,
		Actor:     fmt.Sprintf("open:%s", k.KeyID),
		IP:        clientIP(r),
		Note:      req.Note,
	})
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, map[string]any{"batch_id": res.BatchID, "kamis": res.Kamis, "qty": len(res.Kamis)})
}

func (s *Server) openQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kami string `json:"kami"`
	}
	if !decode(w, r, &req) {
		return
	}
	c, err := s.st.CardByKami(r.Context(), strings.TrimSpace(req.Kami))
	if err != nil {
		writeErr(w, http.StatusNotFound, apierr.CardNotFound, "卡密不存在")
		return
	}
	writeData(w, cardJSON(c))
}

func (s *Server) openExtend(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kami    string `json:"kami"`
		Days    int64  `json:"days"`
		Times   int64  `json:"times"`
		Points  int64  `json:"points"`
		Seconds int64  `json:"seconds"`
	}
	if !decode(w, r, &req) {
		return
	}
	ctx := r.Context()
	c, err := s.st.CardByKami(ctx, strings.TrimSpace(req.Kami))
	if err != nil {
		writeErr(w, http.StatusNotFound, apierr.CardNotFound, "卡密不存在")
		return
	}
	k := openKeyFrom(r)
	err = s.cards.Extend(ctx, c.ID, card.ExtendAdd{
		Days: req.Days, Times: req.Times, Points: req.Points, Seconds: req.Seconds,
	}, card.StatusChange{
		Action: "card.extend",
		Actor:  fmt.Sprintf("open:%s", k.KeyID),
		IP:     clientIP(r),
	})
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, map[string]any{"id": c.ID, "updated": true})
}

func (s *Server) openUnbind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kami string `json:"kami"`
	}
	if !decode(w, r, &req) {
		return
	}
	ctx := r.Context()
	c, err := s.st.CardByKami(ctx, strings.TrimSpace(req.Kami))
	if err != nil {
		writeErr(w, http.StatusNotFound, apierr.CardNotFound, "卡密不存在")
		return
	}
	k := openKeyFrom(r)
	if err := s.cards.Unbind(ctx, c.ID, card.StatusChange{
		Action: "card.unbind",
		Actor:  fmt.Sprintf("open:%s", k.KeyID),
		IP:     clientIP(r),
	}); err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, map[string]any{"id": c.ID, "unbound": true})
}

func (s *Server) openGrant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kami      string `json:"kami"`
		Code      string `json:"code"`
		ExpiresAt int64  `json:"expires_at"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Code == "" {
		writeErr(w, http.StatusBadRequest, apierr.BadInput, "缺少 code")
		return
	}
	ctx := r.Context()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		internalErr(w, s.log, "事务失败", err)
		return
	}
	defer tx.Rollback()

	c, err := s.st.CardByKami(ctx, strings.TrimSpace(req.Kami))
	if err != nil {
		writeErr(w, http.StatusNotFound, apierr.CardNotFound, "卡密不存在")
		return
	}
	ent, err := s.st.EntitlementByCodeTx(ctx, tx, c.AppID, req.Code)
	if err != nil {
		writeErr(w, http.StatusBadRequest, apierr.BadInput, "权限不存在")
		return
	}
	k := openKeyFrom(r)
	if err := s.st.GrantCardEntitlementTx(ctx, tx, c.ID, ent.ID, 0, req.ExpiresAt); err != nil {
		internalErr(w, s.log, "附加权限失败", err)
		return
	}
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     s.now().Unix(),
		Actor:  fmt.Sprintf("open:%s", k.KeyID),
		Action: "card.entitlement_grant",
		Target: fmt.Sprintf("card:%d", c.ID),
		After:  fmt.Sprintf(`{"code":%q,"expires_at":%d}`, req.Code, req.ExpiresAt),
		IP:     clientIP(r),
	}); err != nil {
		internalErr(w, s.log, "写审计失败", err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalErr(w, s.log, "提交失败", err)
		return
	}
	writeData(w, map[string]any{"card_id": c.ID, "code": req.Code, "granted": true})
}

func (s *Server) openOnline(w http.ResponseWriter, r *http.Request) {
	n, err := s.st.ActiveSessionCount(r.Context(), s.now().Unix())
	if err != nil {
		internalErr(w, s.log, "查在线数失败", err)
		return
	}
	writeData(w, map[string]any{"online": n, "server_time": s.now().Unix()})
}

// ---------- 管理端密钥 ----------

func (s *Server) listAPIKeys(w http.ResponseWriter, r *http.Request) {
	list, err := s.st.ListAPIKeys(r.Context())
	if err != nil {
		internalErr(w, s.log, "查密钥失败", err)
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, k := range list {
		var lastUsed any
		if k.LastUsed.Valid {
			lastUsed = k.LastUsed.Int64
		}
		out = append(out, map[string]any{
			"id": k.ID, "name": k.Name, "scopes": k.Scopes, "key_id": k.KeyID,
			"status": k.Status, "last_used": lastUsed, "created_at": k.CreatedAt,
		})
	}
	writeData(w, out)
}

func (s *Server) createAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req struct {
		Name   string `json:"name"`
		Scopes string `json:"scopes"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.Name == "" || req.Scopes == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "name 与 scopes 必填")
		return
	}
	for _, sc := range strings.Split(req.Scopes, ",") {
		switch strings.TrimSpace(sc) {
		case "cards:write", "cards:read", "stats:read":
		default:
			writeErr(w, http.StatusBadRequest, CodeBadInput, "未知权限 "+sc)
			return
		}
	}

	keyRaw := make([]byte, 12)
	secretRaw := make([]byte, 32)
	if _, err := rand.Read(keyRaw); err != nil {
		internalErr(w, s.log, "生成密钥失败", err)
		return
	}
	if _, err := rand.Read(secretRaw); err != nil {
		internalErr(w, s.log, "生成密钥失败", err)
		return
	}
	keyID := "ak_" + hex.EncodeToString(keyRaw)
	secret := base64.RawURLEncoding.EncodeToString(secretRaw)

	id, err := s.st.CreateAPIKey(ctx, store.APIKey{
		Name: req.Name, Scopes: req.Scopes, KeyID: keyID, Secret: secret,
		Status: "active", CreatedAt: s.now().Unix(),
	})
	if err != nil {
		internalErr(w, s.log, "创建密钥失败", err)
		return
	}
	c := adminFrom(r)
	_ = s.writeAudit(ctx, c, "openkey.create", fmt.Sprintf("openkey:%d", id), "",
		fmt.Sprintf(`{"name":%q,"scopes":%q}`, req.Name, req.Scopes), clientIP(r))

	// secret 只在这里返回一次
	writeData(w, map[string]any{
		"id": id, "name": req.Name, "scopes": req.Scopes,
		"key_id": keyID, "secret": secret,
		"hint": "secret 只显示这一次，请立即保存",
	})
}

func (s *Server) setAPIKeyStatus(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := pathID(r)
		if !ok {
			writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
			return
		}
		status := "disabled"
		if enabled {
			status = "active"
		}
		if err := s.st.SetAPIKeyStatus(r.Context(), id, status); err != nil {
			internalErr(w, s.log, "更新密钥失败", err)
			return
		}
		c := adminFrom(r)
		_ = s.writeAudit(r.Context(), c, "openkey.status", fmt.Sprintf("openkey:%d", id), "",
			fmt.Sprintf(`{"status":%q}`, status), clientIP(r))
		writeData(w, map[string]any{"id": id, "status": status})
	}
}
