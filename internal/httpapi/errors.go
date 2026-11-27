package httpapi

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/apierr"
)

// 对外错误码，定义在 apierr，这里只是别名方便 httpapi 内部使用。
const (
	CodeBadInput           = apierr.BadInput
	CodeCardNotFound       = apierr.CardNotFound
	CodeCardExpired        = apierr.CardExpired
	CodeCardBanned         = apierr.CardBanned
	CodeCardExhausted      = apierr.CardExhausted
	CodeDeviceLimit        = apierr.DeviceLimit
	CodeDeviceMismatch     = apierr.DeviceMismatch
	CodeUnauthorized       = apierr.Unauthorized
	CodeRevoked            = apierr.Revoked
	CodePaused             = apierr.Paused
	CodeEntitlementMissing = apierr.EntitlementMissing
	CodeRateLimited        = apierr.RateLimited
	CodeMaintenance        = apierr.Maintenance
	CodeInternal           = apierr.Internal
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
