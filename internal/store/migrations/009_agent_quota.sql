-- 009: 代理额度改成总余额（不再按月重置）
ALTER TABLE agents ADD COLUMN quota_cents INTEGER NOT NULL DEFAULT 0;
UPDATE agents SET quota_cents = monthly_quota_cents;
