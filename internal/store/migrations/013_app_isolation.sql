-- 应用隔离：代理与开放 API 密钥归属应用；历史数据都归到第一个应用。
ALTER TABLE agents ADD COLUMN app_id INTEGER NOT NULL DEFAULT 1;
ALTER TABLE api_keys ADD COLUMN app_id INTEGER NOT NULL DEFAULT 1;
CREATE INDEX idx_agents_app ON agents(app_id);
CREATE INDEX idx_api_keys_app ON api_keys(app_id);
