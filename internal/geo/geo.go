// Package geo 用离线 ip2region.xdb 把 IP 解析成归属地。
// 数据只在进程内查表，不发外部请求，也不落库。
package geo

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"net"
	"strings"
)

//go:embed ip2region.xdb
var xdb []byte

// xdb v2 结构：256 字节头 + 256×256 的向量索引（每格 8 字节）+ 段索引 + 数据区。
// 向量索引给出该 /16 段的段索引起止字节偏移，段索引每 14 字节一条：
// start(4) + end(4) + dataLen(2) + dataPtr(4)。
const (
	headerLen    = 256
	indexCols    = 256
	indexSize    = 8
	segmentIndex = 14
)

// Lookup 返回省份与城市；内网、非法 IP、查不到都返回空串。
func Lookup(ipStr string) (province, city string) {
	province, city, _ = LookupFull(ipStr)
	return province, city
}

// LookupFull 同 Lookup，另外告诉调用方这条记录是不是境外（境外时 province 是国家名）。
func LookupFull(ipStr string) (province, city string, abroad bool) {
	return splitRegion(lookupRaw(ipStr))
}

// lookupRaw 返回库里的原始记录，形如 "中国|0|江苏省|南京市|电信"。
func lookupRaw(ipStr string) string {
	ip := net.ParseIP(strings.TrimSpace(ipStr))
	if ip == nil {
		return ""
	}
	v4 := ip.To4()
	if v4 == nil || len(xdb) < headerLen {
		return ""
	}
	idx := (int(v4[0])*indexCols + int(v4[1])) * indexSize
	off := headerLen + idx
	if off+indexSize > len(xdb) {
		return ""
	}
	sPtr := int(binary.LittleEndian.Uint32(xdb[off:]))
	ePtr := int(binary.LittleEndian.Uint32(xdb[off+4:]))
	// 该段没有数据（sPtr/ePtr 为 0）时直接返回
	if sPtr == 0 || ePtr == 0 || sPtr >= len(xdb) {
		return ""
	}

	l, h := 0, (ePtr-sPtr)/segmentIndex
	for l <= h {
		m := (l + h) >> 1
		p := sPtr + m*segmentIndex
		if p+segmentIndex > len(xdb) {
			return ""
		}
		switch {
		case bytes.Compare(v4, xdb[p:p+4]) < 0:
			h = m - 1
		case bytes.Compare(v4, xdb[p+4:p+8]) > 0:
			l = m + 1
		default:
			dataLen := int(binary.LittleEndian.Uint16(xdb[p+8:]))
			dataPtr := int(binary.LittleEndian.Uint32(xdb[p+10:]))
			if dataLen == 0 || dataPtr < 0 || dataPtr+dataLen > len(xdb) {
				return ""
			}
			return string(xdb[dataPtr : dataPtr+dataLen])
		}
	}
	return ""
}

// splitRegion 拆分 "国家|省份|城市|运营商|国家码"；国内返回省份+城市，国外返回国家名。
func splitRegion(region string) (province, city string, abroad bool) {
	if region == "" {
		return "", "", false
	}
	parts := strings.Split(region, "|")
	get := func(i int) string {
		if i < len(parts) {
			return strings.TrimSpace(parts[i])
		}
		return ""
	}
	country, province, city := get(0), get(1), get(2)
	if province == "0" {
		province = ""
	}
	if city == "0" {
		city = ""
	}
	switch country {
	case "", "中国", "CN":
		return province, city, false
	case "Reserved", "0":
		return "", "", false
	default:
		// 国外只按国家统计，城市名对图表没意义
		return country, "", true
	}
}

// ShortProvince 把「广东省 / 内蒙古自治区 / 北京市」压成用于图表的两字名。
func ShortProvince(p string) string {
	if p == "" {
		return ""
	}
	for _, suffix := range []string{"维吾尔自治区", "回族自治区", "壮族自治区", "特别行政区", "自治区", "省", "市"} {
		p = strings.TrimSuffix(p, suffix)
	}
	return p
}
