package main

import (
	"testing"
	"time"
)

func TestNextAnchorTime(t *testing.T) {
	loc := time.UTC
	cases := []struct{ now, want time.Time }{
		{time.Date(2026, 11, 27, 0, 0, 0, 0, loc), time.Date(2026, 11, 27, 0, 5, 0, 0, loc)},
		{time.Date(2026, 11, 27, 0, 5, 0, 0, loc), time.Date(2026, 11, 28, 0, 5, 0, 0, loc)},
		{time.Date(2026, 11, 27, 15, 30, 0, 0, loc), time.Date(2026, 11, 28, 0, 5, 0, 0, loc)},
	}
	for _, tc := range cases {
		if got := nextAnchorTime(tc.now); !got.Equal(tc.want) {
			t.Errorf("now=%v got=%v want=%v", tc.now, got, tc.want)
		}
	}
}
