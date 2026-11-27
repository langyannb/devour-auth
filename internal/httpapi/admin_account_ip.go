package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"

	"github.com/langyannb/devour-auth/internal/store"
)

// parseIPAllow 解析「逗号或空格分隔的 IP / CIDR」，返回规范化文本与匹配用网段。
// 空串表示不限；任何一项非法都整单拒绝。
func parseIPAllow(s string) (string, []netip.Prefix, error) {
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == ' ' || r == '\t' || r == '\n' || r == '\r'
	})
	if len(fields) == 0 {
		return "", nil, nil
	}
	out := make([]string, 0, len(fields))
	prefixes := make([]netip.Prefix, 0, len(fields))
	for _, f := range fields {
		if p, err := netip.ParsePrefix(f); err == nil {
			p = p.Masked()
			prefixes = append(prefixes, p)
			out = append(out, p.String())
			continue
		}
		addr, err := netip.ParseAddr(f)
		if err != nil {
			return "", nil, fmt.Errorf("「%s」不是合法的 IP 或 CIDR", f)
		}
		addr = addr.Unmap()
		prefixes = append(prefixes, netip.PrefixFrom(addr, addr.BitLen()))
		out = append(out, addr.String())
	}
	return strings.Join(out, ","), prefixes, nil
}

// ipAllowContains 精确匹配单个 IP 或声明的 CIDR，不做 /24 容忍（design.md 16.4）。
func ipAllowContains(prefixes []netip.Prefix, ip string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(ip))
	if err != nil {
		return false
	}
	addr = addr.Unmap()
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// setAccountIPAllow 设置管理员的登录 IP 白名单。
func (s *Server) setAccountIPAllow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "id 不合法")
		return
	}
	var req struct {
		IPAllow string `json:"ip_allow"`
	}
	if !decode(w, r, &req) {
		return
	}
	normalized, prefixes, err := parseIPAllow(req.IPAllow)
	if err != nil {
		writeErr(w, http.StatusBadRequest, CodeBadInput, err.Error())
		return
	}
	before, err := s.st.AdminAccountByID(ctx, id)
	if err == nil && adminScopeID(r) > 0 && before.AppID != adminScopeID(r) {
		writeErr(w, http.StatusNotFound, CodeBadInput, "账号不存在")
		return
	}
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, CodeBadInput, "账号不存在")
		return
	} else if err != nil {
		internalErr(w, s.log, "查账号失败", err)
		return
	}

	c := adminFrom(r)
	// 不允许把自己锁在门外：改的是当前账号时，新名单必须包含当前 IP。
	if id == c.Admin.ID && normalized != "" && !ipAllowContains(prefixes, clientIP(r)) {
		writeErr(w, http.StatusBadRequest, CodeBadInput, "新白名单不包含当前登录 IP，保存会把自己锁在外面")
		return
	}

	if err := s.st.SetAdminIPAllow(ctx, id, normalized); err != nil {
		internalErr(w, s.log, "保存 IP 白名单失败", err)
		return
	}
	a, err := s.st.AdminAccountByID(ctx, id)
	if err != nil {
		internalErr(w, s.log, "查账号失败", err)
		return
	}
	_ = s.writeAudit(ctx, c, "admin.set_ip_allow", fmt.Sprintf("admin:%d", id),
		jsonOf(map[string]string{"ip_allow": before.IPAllow}),
		jsonOf(map[string]string{"ip_allow": a.IPAllow}), clientIP(r))
	writeData(w, accountRow(a))
}
