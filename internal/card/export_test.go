package card

import "time"

// 让外部测试包控制时钟。
func (s *Service) NowUnixForTest() int64 { return s.now().Unix() }

func (s *Service) SetNowForTest(unix int64) {
	s.now = func() time.Time { return time.Unix(unix, 0) }
}
