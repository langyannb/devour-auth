-- 管理员账号可以绑定一个应用；0 = 全局（能看所有应用）
ALTER TABLE admins ADD COLUMN app_id INTEGER NOT NULL DEFAULT 0;
