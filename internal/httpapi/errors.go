package httpapi

import (
	"encoding/json"
	"net/http"
	"time"
)

// 对外错误码，design.md 第 12 节。不把内部错误透传给客户端。
const (
	CodeBadInput           = "BAD_INPUT"
	CodeCardNotFound       = "CARD_NOT_FOUND"
	CodeCardExpired        = "CARD_EXPIRED"
	CodeCardBanned         = "CARD_BANNED"
	CodeCardExhausted      = "CARD_EXHAUSTED"
	CodeDeviceLimit        = "DEVICE_LIMIT"
	CodeDeviceMismatch     = "DEVICE_MISMATCH"
	CodeUnauthorized       = "UNAUTHORIZED"
	CodeRevoked            = "REVOKED"
	CodePaused             = "PAUSED"
	CodeEntitlementMissing = "ENTITLEMENT_MISSING"
	CodeRateLimited        = "RATE_LIMITED"
	CodeMaintenance        = "MAINTENANCE"
	CodeInternal           = "INTERNAL"
)

type errorBody struct {
	OK         bool   `json:"ok"`
	Code       string `json:"code"`
	Message    string `json:"message"`
	ServerTime int64  `json:"server_time"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeData(w http.ResponseWriter, v any) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": v})
}

func writeErr(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, errorBody{
		OK:         false,
		Code:       code,
		Message:    msg,
		ServerTime: time.Now().Unix(),
	})
}
