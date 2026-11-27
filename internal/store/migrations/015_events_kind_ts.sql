-- 数据看板的省份统计按 kind + 时间窗扫 events，补复合索引
CREATE INDEX IF NOT EXISTS idx_events_kind_ts ON events(kind, ts);
