package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// openDebug 用指定密钥在后端签一次名并发到本地开放接口，供「调试助手」使用。
// 这样 secret 不用暴露给浏览器，又能验证签名链路。
func (s *Server) openDebug(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyID  string `json:"key_id"`
		Method string `json:"method"`
		Path   string `json:"path"`
		Body   string `json:"body"`
	}
	if !decode(w, r, &req) {
		return
	}
	req.Method = strings.ToUpper(strings.TrimSpace(req.Method))
	if req.Method == "" {
		req.Method = http.MethodPost
	}
	if !strings.HasPrefix(req.Path, "/open/v1/") {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "path 必须以 /open/v1/ 开头")
		return
	}
	k, err := s.st.APIKeyByKeyID(r.Context(), req.KeyID)
	if err != nil {
		writeErr(w, http.StatusNotFound, CodeBadInput, "密钥不存在")
		return
	}
	if k.Status != "active" {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "密钥已停用")
		return
	}

	now := s.now().Unix()
	ts := strconv.FormatInt(now, 10)
	rawNonce := make([]byte, 16)
	if _, err := rand.Read(rawNonce); err != nil {
		internalErr(w, s.log, "生成 nonce 失败", err)
		return
	}
	nonce := hex.EncodeToString(rawNonce)
	sig := sign(k.Secret, req.Method, req.Path, ts, nonce, []byte(req.Body))

	url := "http://" + s.listenAddr + req.Path
	hr, err := http.NewRequestWithContext(r.Context(), req.Method, url, strings.NewReader(req.Body))
	if err != nil {
		internalErr(w, s.log, "构造请求失败", err)
		return
	}
	if req.Body != "" {
		hr.Header.Set("Content-Type", "application/json")
	}
	hr.Header.Set("X-Api-Key", k.KeyID)
	hr.Header.Set("X-Timestamp", ts)
	hr.Header.Set("X-Nonce", nonce)
	hr.Header.Set("X-Signature", sig)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(hr)
	if err != nil {
		internalErr(w, s.log, "调试请求失败", err)
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))

	writeData(w, map[string]any{
		"status":   resp.StatusCode,
		"body":     string(raw),
		"request":  fmt.Sprintf("%s %s\nX-Api-Key: %s\nX-Timestamp: %s\nX-Nonce: %s\nX-Signature: %s", req.Method, req.Path, k.KeyID, ts, nonce, sig),
		"used_key": map[string]any{"name": k.Name, "scopes": k.Scopes, "key_id": k.KeyID},
	})
}
