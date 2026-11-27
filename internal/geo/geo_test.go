package geo

import "testing"

// 库是离线的，公网 IP 的归属地不会变，断言省份即可。
func TestLookup(t *testing.T) {
	cases := map[string]string{
		"114.114.114.114": "江苏省",
		"223.5.5.5":       "浙江省",
		"42.48.94.28":     "湖南省",
	}
	for ip, want := range cases {
		province, city := Lookup(ip)
		if province != want {
			t.Fatalf("Lookup(%s) 省份 = %q, want %q (city=%q)", ip, province, want, city)
		}
		if ShortProvince(province) == "" {
			t.Fatalf("ShortProvince(%q) 为空", province)
		}
	}

	// 内网/保留地址不能当地理位置
	for _, ip := range []string{"127.0.0.1", "192.168.1.1", "10.0.0.1", "not-an-ip"} {
		if p, c := Lookup(ip); p != "" || c != "" {
			t.Fatalf("Lookup(%s) = %q %q, want empty", ip, p, c)
		}
	}

	// 国外按国家名返回
	if p, _ := Lookup("8.8.8.8"); p == "" {
		t.Fatal("国外 IP 应返回国家名")
	}
}

func TestShortProvince(t *testing.T) {
	cases := map[string]string{
		"广东省":      "广东",
		"内蒙古自治区":   "内蒙古",
		"新疆维吾尔自治区": "新疆",
		"北京市":      "北京",
	}
	for in, want := range cases {
		if got := ShortProvince(in); got != want {
			t.Fatalf("ShortProvince(%q) = %q, want %q", in, got, want)
		}
	}
}
