package httpapi

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"time"

	"github.com/langyannb/devour-auth/internal/card"
)

// unbindTicket 查卡成功后签发，用于自助解绑（5 分钟），避免解绑再要一次验证码。
type unbindTicket struct {
	Card int64 `json:"card"`
	Exp  int64 `json:"exp"`
}

func (s *Server) signUnbindTicket(cardID int64) string {
	t := unbindTicket{Card: cardID, Exp: s.now().Add(5 * time.Minute).Unix()}
	body, _ := json.Marshal(t)
	sig := ed25519.Sign(s.ticketKey, body)
	return base64.RawURLEncoding.EncodeToString(body) + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func (s *Server) verifyUnbindTicket(tok string) (int64, bool) {
	var t unbindTicket
	for i := 0; i < len(tok); i++ {
		if tok[i] != '.' {
			continue
		}
		body, err := base64.RawURLEncoding.DecodeString(tok[:i])
		if err != nil {
			return 0, false
		}
		sig, err := base64.RawURLEncoding.DecodeString(tok[i+1:])
		if err != nil || !ed25519.Verify(s.ticketKey.Public().(ed25519.PublicKey), body, sig) {
			return 0, false
		}
		if err := json.Unmarshal(body, &t); err != nil {
			return 0, false
		}
		if t.Card <= 0 || t.Exp < s.now().Unix() {
			return 0, false
		}
		return t.Card, true
	}
	return 0, false
}

// unbindSettings 管理员在 flags 里配的解绑策略；没配就用卡快照里的旧策略。
type unbindSettings struct {
	Free         int64
	CooldownDays int64
	CostHours    int64
	RejectPaid   bool
	Configured   bool
}

func (s *Server) unbindSettings(flags map[string]string, snap card.Snapshot) unbindSettings {
	out := unbindSettings{
		Free:         snap.RebindFree,
		CooldownDays: snap.RebindCooldownDays,
		CostHours:    snap.RebindCostHours,
		RejectPaid:   snap.RebindCostHours <= 0,
	}
	if v, ok := flags["unbind_free"]; ok {
		out.Free = parseInt64(v)
		out.Configured = true
	}
	if v, ok := flags["unbind_cooldown_days"]; ok {
		out.CooldownDays = parseInt64(v)
		out.Configured = true
	}
	if v, ok := flags["unbind_mode"]; ok {
		out.RejectPaid = v != "charge"
		out.Configured = true
	}
	if v, ok := flags["unbind_cost_hours"]; ok {
		out.CostHours = parseInt64(v)
		out.Configured = true
	}
	return out
}

func parseInt64(v string) int64 {
	var n int64
	neg := false
	for i := 0; i < len(v); i++ {
		if i == 0 && v[i] == '-' {
			neg = true
			continue
		}
		if v[i] < '0' || v[i] > '9' {
			return 0
		}
		n = n*10 + int64(v[i]-'0')
	}
	if neg {
		n = -n
	}
	return n
}

// maskFingerprint 只露后 4 位，解绑页不暴露完整设备指纹。
func maskFingerprint(fp string) string {
	if len(fp) <= 4 {
		return "****"
	}
	return "****" + fp[len(fp)-4:]
}
