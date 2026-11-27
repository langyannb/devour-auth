package money

import "testing"

func TestYuan(t *testing.T) {
	cases := map[int64]string{
		0:     "0.00",
		300:   "3.00",
		1999:  "19.99",
		77700: "777.00",
		-1:    "-0.01",
	}
	for cents, want := range cases {
		if got := Yuan(cents); got != want {
			t.Fatalf("Yuan(%d) = %s, want %s", cents, got, want)
		}
	}
}
