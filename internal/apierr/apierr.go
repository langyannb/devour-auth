// Package apierr 定义对外错误码（design.md 第 12 节）与可携带错误码的 error。
// 业务包只依赖这里，不依赖 httpapi。
package apierr

import "fmt"

const (
	BadInput           = "BAD_INPUT"
	CardNotFound       = "CARD_NOT_FOUND"
	CardExpired        = "CARD_EXPIRED"
	CardBanned         = "CARD_BANNED"
	CardRecycled       = "CARD_RECYCLED"
	CardExhausted      = "CARD_EXHAUSTED"
	DeviceLimit        = "DEVICE_LIMIT"
	DeviceMismatch     = "DEVICE_MISMATCH"
	Unauthorized       = "UNAUTHORIZED"
	Revoked            = "REVOKED"
	Paused             = "PAUSED"
	EntitlementMissing = "ENTITLEMENT_MISSING"
	RateLimited        = "RATE_LIMITED"
	Maintenance        = "MAINTENANCE"
	Internal           = "INTERNAL"
)

type Error struct {
	Code    string
	Message string
}

func (e *Error) Error() string { return e.Message }

func New(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}
