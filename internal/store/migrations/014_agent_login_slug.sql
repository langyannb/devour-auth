-- 下级代理专属登录链接：/agent/{slug}
ALTER TABLE agents ADD COLUMN login_slug TEXT;
ALTER TABLE agents ADD COLUMN password_set INTEGER NOT NULL DEFAULT 1;
CREATE UNIQUE INDEX idx_agents_login_slug ON agents(login_slug) WHERE login_slug IS NOT NULL;
