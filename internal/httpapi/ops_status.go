package httpapi

import (
	"net/http"
	"os"
	"strings"
	"time"
)

type opsTask struct {
	LastAt int64 `json:"last_at"`
	NextAt int64 `json:"next_at"`
}

type opsBackup struct {
	LastAt     int64 `json:"last_at"`
	NextAt     int64 `json:"next_at"`
	Configured bool  `json:"configured"`
}

// opsStatus 定时任务上次/下次执行时间。
// 周期以代码和 deploy/*.timer 为准：锚定 00:05、日统计 00:07、账单每月 1 日 01:10、备份每小时。
func (s *Server) opsStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	now := s.now()

	out := struct {
		Anchor opsTask   `json:"anchor"`
		Rollup opsTask   `json:"rollup"`
		Bills  opsTask   `json:"bills"`
		Backup opsBackup `json:"backup"`
	}{}

	var day string
	if err := s.st.DB().QueryRowContext(ctx,
		`SELECT day FROM audit_anchor ORDER BY day DESC LIMIT 1`).Scan(&day); err == nil {
		out.Anchor.LastAt = dayUnix(day, "2006-01-02")
	}
	out.Anchor.NextAt = nextDaily(now, 0, 5)

	if err := s.st.DB().QueryRowContext(ctx,
		`SELECT COALESCE(MAX(day), '') FROM daily_stats`).Scan(&day); err == nil {
		out.Rollup.LastAt = dayUnix(day, "2006-01-02")
	}
	out.Rollup.NextAt = nextDaily(now, 0, 7)

	var month string
	if err := s.st.DB().QueryRowContext(ctx,
		`SELECT month FROM agent_bills ORDER BY month DESC LIMIT 1`).Scan(&month); err == nil {
		out.Bills.LastAt = dayUnix(month, "2006-01")
	}
	out.Bills.NextAt = nextMonthly(now, 1, 1, 10)

	out.Backup = s.backupStatus(now)
	writeData(w, out)
}

// backupStatus 读备份目录里最新的 *.db.gpg；未配置或目录不存在时视为未接入。
func (s *Server) backupStatus(now time.Time) opsBackup {
	if s.backupDir == "" {
		return opsBackup{}
	}
	if st, err := os.Stat(s.backupDir); err != nil || !st.IsDir() {
		return opsBackup{}
	}
	entries, _ := os.ReadDir(s.backupDir)
	out := opsBackup{NextAt: nextHourly(now), Configured: true}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".db.gpg") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if mt := info.ModTime().Unix(); mt > out.LastAt {
			out.LastAt = mt
		}
	}
	return out
}

// dayUnix 把 "2006-01-02" 或 "2006-01" 文本按服务器本地时区转 Unix 秒。
func dayUnix(v, layout string) int64 {
	t, err := time.ParseInLocation(layout, v, time.Local)
	if err != nil {
		return 0
	}
	return t.Unix()
}

// nextDaily 下一个本地时间的 hour:min。
func nextDaily(now time.Time, hour, min int) int64 {
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, min, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 0, 1)
	}
	return t.Unix()
}

// nextMonthly 下一个本地时间的每月 day 日 hour:min。
func nextMonthly(now time.Time, day, hour, min int) int64 {
	t := time.Date(now.Year(), now.Month(), day, hour, min, 0, 0, now.Location())
	if !t.After(now) {
		t = t.AddDate(0, 1, 0)
	}
	return t.Unix()
}

// nextHourly 下一个本地整点。
func nextHourly(now time.Time) int64 {
	return time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, now.Location()).
		Add(time.Hour).Unix()
}
