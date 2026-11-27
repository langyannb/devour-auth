package httpapi

import "net"

// sameSubnet 判断两个 IP 是否同一网段（IPv4 /24、IPv6 /48）。
// 登录会话绑定到网段而不是精确 IP：手机切基站、NAT 出口轮换不会把用户踢下线，
// 但换了网络的伪造 cookie 依然失效。
func sameSubnet(a, b string) bool {
	if a == "" || b == "" {
		return true
	}
	if a == b {
		return true
	}
	ia, ib := net.ParseIP(a), net.ParseIP(b)
	if ia == nil || ib == nil {
		return false
	}
	if ia.To4() != nil && ib.To4() != nil {
		return ia.Mask(net.CIDRMask(24, 32)).Equal(ib.Mask(net.CIDRMask(24, 32)))
	}
	return ia.Mask(net.CIDRMask(48, 128)).Equal(ib.Mask(net.CIDRMask(48, 128)))
}
