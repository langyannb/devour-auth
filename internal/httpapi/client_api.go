package httpapi

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/langyannb/devour-auth/internal/apierr"
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
