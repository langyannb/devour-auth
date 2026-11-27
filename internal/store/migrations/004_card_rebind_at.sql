-- 004: 记录最近一次换绑时间，用于策略里的换绑冷却
ALTER TABLE cards ADD COLUMN rebind_at INTEGER NOT NULL DEFAULT 0;
