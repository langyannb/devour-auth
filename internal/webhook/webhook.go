// Package webhook 把卡密事件异步推给第三方 URL。
package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/langyannb/devour-auth/internal/store"
)

type Dispatcher struct {
	st     *store.Store
	log    *slog.Logger
	client *http.Client
	// 最多尝试次数与每次重试前的等待，测试可注入零退避。
	attempts int
	backoff  []time.Duration
}

func New(st *store.Store, log *slog.Logger) *Dispatcher {
	return &Dispatcher{
		st:       st,
		log:      log,
		client:   &http.Client{Timeout: 5 * time.Second},
		attempts: 3,
		backoff:  []time.Duration{time.Second, 2 * time.Second},
	}
}

// Dispatch 异步推送；每次投递的结果落 webhook_deliveries，不影响业务。
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

	start := time.Now()
	var (
		status   int
		lastErr  string
		attempts int
	)
	for i := 0; i < d.attempts; i++ {
		if i > 0 && i-1 < len(d.backoff) {
			time.Sleep(d.backoff[i-1])
		}
		attempts++
		status = 0
		// 独立超时，且不挂在请求 context 上：请求结束仍要投递完成。
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.URL, bytes.NewReader(body))
		if err != nil {
			cancel()
			lastErr = err.Error()
			break
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Devour-Event", event)
		req.Header.Set("X-Devour-Signature", sig)

		resp, err := d.client.Do(req)
		if err != nil {
			cancel()
			lastErr = err.Error()
			continue
		}
		status = resp.StatusCode
		io.Copy(io.Discard, resp.Body) // 读完以便复用连接
		resp.Body.Close()
		cancel()
		if status >= 200 && status < 300 {
			lastErr = ""
			break
		}
		lastErr = fmt.Sprintf("HTTP %d", status)
	}

	ok := status >= 200 && status < 300
	if !ok {
		d.log.Warn("webhook 投递失败", "id", h.ID, "url", h.URL, "attempts", attempts, "err", lastErr)
	}
	if err := d.st.CreateWebhookDelivery(context.Background(), store.WebhookDelivery{
		WebhookID:  h.ID,
		Event:      event,
		URL:        h.URL,
		Attempts:   attempts,
		OK:         ok,
		StatusCode: status,
		Error:      lastErr,
		CreatedAt:  start.Unix(),
		FinishedAt: time.Now().Unix(),
	}); err != nil {
		d.log.Warn("webhook 投递记录写入失败", "id", h.ID, "err", err)
	}
}
