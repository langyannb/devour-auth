package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/langyannb/devour-auth/internal/apierr"
	"github.com/langyannb/devour-auth/internal/audit"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/clientauth"
)

// writeAPIError 把业务错误映射成对外统一错误体；未知错误一律 INTERNAL，不透传内部信息。
func (s *Server) writeAPIError(w http.ResponseWriter, err error) {
	var ae *apierr.Error
	if errors.As(err, &ae) {
		status := http.StatusBadRequest
		switch ae.Code {
		case apierr.CardNotFound:
			status = http.StatusNotFound
		case apierr.CardExpired, apierr.CardBanned, apierr.CardExhausted,
			apierr.DeviceLimit, apierr.DeviceMismatch, apierr.EntitlementMissing:
			status = http.StatusForbidden
		case apierr.Unauthorized, apierr.Revoked:
			status = http.StatusUnauthorized
		case apierr.RateLimited:
			status = http.StatusTooManyRequests
		case apierr.Paused, apierr.Maintenance:
			status = http.StatusConflict
		case apierr.Internal:
			status = http.StatusInternalServerError
		}
		writeErr(w, status, ae.Code, ae.Message)
		return
	}
	s.log.Error("客户端接口内部错误", "err", err)
	writeErr(w, http.StatusInternalServerError, CodeInternal, "服务内部错误")
}

func bearer(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if len(h) > 7 && strings.EqualFold(h[:7], "bearer ") {
		return strings.TrimSpace(h[7:])
	}
	return ""
}

type clientLoginReq struct {
	Kami   string `json:"kami"`
	Device struct {
		Fingerprint string `json:"fingerprint"`
		Model       string `json:"model"`
		Brand       string `json:"brand"`
		OS          string `json:"os"`
		AppVersion  string `json:"app_version"`
	} `json:"device"`
	TS    int64  `json:"ts"`
	Nonce string `json:"nonce"`
}

func (s *Server) clientLogin(w http.ResponseWriter, r *http.Request) {
	var req clientLoginReq
	if !decode(w, r, &req) {
		return
	}
	ip := clientIP(r)
	// 登录限流：5 次/分/IP + 20 次/时/卡密（design.md 16.4）
	if !s.lim.Allow("client:login:ip:"+ip, 5, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "尝试过于频繁")
		return
	}
	if !s.lim.Allow("client:login:card:"+req.Kami, 20, time.Hour) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "尝试过于频繁")
		return
	}
	res, err := s.clients.Login(r.Context(), clientauth.LoginReq{
		Kami:        strings.TrimSpace(req.Kami),
		Fingerprint: req.Device.Fingerprint,
		Model:       req.Device.Model,
		Brand:       req.Device.Brand,
		OS:          req.Device.OS,
		AppVersion:  req.Device.AppVersion,
		TS:          req.TS,
		Nonce:       req.Nonce,
	}, ip, truncate(r.UserAgent(), 200))
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, res)
}

func (s *Server) clientRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &req) {
		return
	}
	if req.RefreshToken == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "缺少 refresh_token")
		return
	}
	res, err := s.clients.Refresh(r.Context(), req.RefreshToken, clientIP(r), truncate(r.UserAgent(), 200))
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, res)
}

func (s *Server) clientLogout(w http.ResponseWriter, r *http.Request) {
	tok := bearer(r)
	if tok == "" {
		writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "缺少访问令牌")
		return
	}
	if err := s.clients.Logout(r.Context(), tok); err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, map[string]any{"logged_out": true})
}

// ---------- 消耗与暂停 ----------

func (s *Server) clientConsume(w http.ResponseWriter, r *http.Request) {
	tok := bearer(r)
	if tok == "" {
		writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "缺少访问令牌")
		return
	}
	var req struct {
		EntitlementCode string `json:"entitlement_code"`
		Nonce           string `json:"nonce"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := s.clients.Consume(r.Context(), tok, req.EntitlementCode, req.Nonce,
		clientIP(r), truncate(r.UserAgent(), 200))
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, res)
}

func (s *Server) clientPause(w http.ResponseWriter, r *http.Request) {
	tok := bearer(r)
	if tok == "" {
		writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "缺少访问令牌")
		return
	}
	var req struct {
		Paused bool `json:"paused"`
	}
	if !decode(w, r, &req) {
		return
	}
	res, err := s.clients.Pause(r.Context(), tok, req.Paused,
		clientIP(r), truncate(r.UserAgent(), 200))
	if err != nil {
		s.writeAPIError(w, err)
		return
	}
	writeData(w, res)
}

// ---------- 单卡权限（管理端） ----------

type cardEntReq struct {
	Code      string `json:"code"`
	ExpiresAt int64  `json:"expires_at"`
}

func (s *Server) grantCardEntitlement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req cardEntReq
	if !decode(w, r, &req) {
		return
	}
	if req.Code == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "缺少 code")
		return
	}
	ctx := r.Context()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		internalErr(w, s.log, "事务失败", err)
		return
	}
	defer tx.Rollback()

	cardRow, err := s.st.CardTx(ctx, tx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "卡不存在")
		return
	}
	ent, err := s.st.EntitlementByCodeTx(ctx, tx, cardRow.AppID, req.Code)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "权限不存在")
		return
	}
	c := adminFrom(r)
	if err := s.st.GrantCardEntitlementTx(ctx, tx, id, ent.ID, c.Admin.ID, req.ExpiresAt); err != nil {
		internalErr(w, s.log, "附加权限失败", err)
		return
	}
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     s.now().Unix(),
		Actor:  fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
		Action: "card.entitlement_grant",
		Target: fmt.Sprintf("card:%d", id),
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
	writeData(w, map[string]any{"card_id": id, "code": req.Code, "granted": true})
}

func (s *Server) revokeCardEntitlement(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req cardEntReq
	if !decode(w, r, &req) {
		return
	}
	if req.Code == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "缺少 code")
		return
	}
	ctx := r.Context()
	tx, err := s.st.DB().BeginTx(ctx, nil)
	if err != nil {
		internalErr(w, s.log, "事务失败", err)
		return
	}
	defer tx.Rollback()

	cardRow, err := s.st.CardTx(ctx, tx, id)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "卡不存在")
		return
	}
	ent, err := s.st.EntitlementByCodeTx(ctx, tx, cardRow.AppID, req.Code)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "权限不存在")
		return
	}
	c := adminFrom(r)
	if err := s.st.RevokeCardEntitlementTx(ctx, tx, id, ent.ID, c.Admin.ID, s.now().Unix()); err != nil {
		internalErr(w, s.log, "撤销权限失败", err)
		return
	}
	if err := audit.Append(ctx, tx, audit.Entry{
		TS:     s.now().Unix(),
		Actor:  fmt.Sprintf("%s:%d", c.Admin.Role, c.Admin.ID),
		Action: "card.entitlement_revoke",
		Target: fmt.Sprintf("card:%d", id),
		Before: fmt.Sprintf(`{"code":%q}`, req.Code),
		After:  fmt.Sprintf(`{"code":%q,"revoked":true}`, req.Code),
		IP:     clientIP(r),
	}); err != nil {
		internalErr(w, s.log, "写审计失败", err)
		return
	}
	if err := tx.Commit(); err != nil {
		internalErr(w, s.log, "提交失败", err)
		return
	}
	writeData(w, map[string]any{"card_id": id, "code": req.Code, "revoked": true})
}

// ---------- 查卡（终端用户自助查询） ----------

// clientQuery 凭卡密 + 图形验证码查状态与绑定数，不返回卡密本身，也不需要登录。
func (s *Server) clientQuery(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kami        string `json:"kami"`
		CaptchaID   string `json:"captcha_id"`
		CaptchaCode string `json:"captcha_code"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Kami) == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "请输入卡密")
		return
	}
	// 验证码先于查卡限流：错码不占用正常用户的查询额度。
	if req.CaptchaID == "" || req.CaptchaCode == "" || !s.captchaVerify(req.CaptchaID, req.CaptchaCode) {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "验证码错误")
		return
	}
	ip := clientIP(r)
	if !s.lim.Allow("query:ip:"+ip, 10, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "查询过于频繁")
		return
	}

	c, err := s.st.CardByKami(r.Context(), strings.TrimSpace(req.Kami))
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeCardNotFound, "卡密不存在")
		return
	}
	bound, err := s.st.CountBoundDevices(r.Context(), c.ID)
	if err != nil {
		internalErr(w, s.log, "查设备失败", err)
		return
	}
	snap := clientauth.SnapshotOf(c)
	writeData(w, map[string]any{
		"status":            c.Status,
		"kind":              c.Kind,
		"duration_seconds":  snap.TotalSeconds(),
		"expire_at":         c.ExpireAt,
		"points":            c.Points,
		"remain_times":      c.RemainTimes,
		"remaining_seconds": c.RemainingSeconds,
		"paused":            c.Paused == 1,
		"device_limit":      snap.DeviceLimit,
		"bound_devices":     bound,
		"server_time":       s.now().Unix(),
	})
}

// clientQueryUnbind 终端用户自助解绑：受策略的免费换绑次数限制。
func (s *Server) clientQueryUnbind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kami string `json:"kami"`
	}
	if !decode(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Kami) == "" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "请输入卡密")
		return
	}
	ip := clientIP(r)
	if !s.lim.Allow("query:unbind:"+ip, 5, time.Minute) {
		writeErr(w, http.StatusTooManyRequests, CodeRateLimited, "操作过于频繁")
		return
	}
	ctx := r.Context()
	c, err := s.st.CardByKami(ctx, strings.TrimSpace(req.Kami))
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeCardNotFound, "卡密不存在")
		return
	}
	err = s.cards.SelfUnbind(ctx, c.ID, card.StatusChange{
		Action: "card.self_unbind",
		Actor:  "user:query",
		IP:     ip,
	})
	if err != nil {
		if errors.Is(err, card.ErrState) {
			writeErr(w, http.StatusConflict, CodeBadInput, err.Error())
			return
		}
		internalErr(w, s.log, "自助解绑失败", err)
		return
	}
	writeData(w, map[string]any{"unbound": true})
}

func (s *Server) clientSession(w http.ResponseWriter, r *http.Request) {
	tok := bearer(r)
	if tok == "" {
		writeErr(w, http.StatusUnauthorized, CodeUnauthorized, "缺少访问令牌")
		return
	}
	res, err := s.clients.Status(r.Context(), tok, clientIP(r), truncate(r.UserAgent(), 200))
	if err != nil {
		// PAUSED 是状态而不是失败：带上卡余额，客户端据此关闭全部功能。
		var ae *apierr.Error
		if errors.As(err, &ae) && ae.Code == apierr.Paused {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok": false, "code": ae.Code, "message": ae.Message,
				"data": res, "server_time": res.ServerTime,
			})
			return
		}
		s.writeAPIError(w, err)
		return
	}
	writeData(w, res)
}
