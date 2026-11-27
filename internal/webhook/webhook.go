// Package webhook 把卡密事件异步推给第三方 URL。
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/store"
)

type Dispatcher struct {
	st     *store.Store
	log    *slog.Logger
	client *http.Client
}

func New(st *store.Store, log *slog.Logger) *Dispatcher {
	return &Dispatcher{st: st, log: log, client: &http.Client{Timeout: 5 * time.Second}}
}

// Dispatch 异步推送；失败只记日志，不影响业务。
func (d *Dispatcher) Dispatch(ctx context.Context, appID int64, event string, payload map[string]any) {
	if appID <= 0 {
		return
	}
	hooks, err := d.st.WebhooksFor(ctx, appID, event)
	if err != nil || len(hooks) == 0 {
		return
	}
	body, err := json.Marshal(map[string]any{
		"event": event,
		"ts":    time.Now().Unix(),
		"data":  payload,
	})
	if err != nil {
		return
	}
	for _, h := range hooks {
		go d.send(h, event, body)
	}
}

func (d *Dispatcher) send(h store.Webhook, event string, body []byte) {
	mac := hmac.New(sha256.New, []byte(h.Secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req, err := http.NewRequest(http.MethodPost, h.URL, bytes.NewReader(body))
	if err != nil {
		d.log.Warn("webhook 构造失败", "id", h.ID, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Devour-Event", event)
	req.Header.Set("X-Devour-Signature", sig)

	resp, err := d.client.Do(req)
	if err != nil {
		d.log.Warn("webhook 投递失败", "id", h.ID, "url", h.URL, "err", err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		d.log.Warn("webhook 返回非 2xx", "id", h.ID, "status", resp.StatusCode)
	}
}
