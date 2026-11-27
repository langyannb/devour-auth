-- 产品只保留时长卡/永久卡：次数、点数、计时、试用卡不再发放。
-- 历史卡与其策略保留（外键与售后需要），只是策略置为停用，不再出现在可选列表。
UPDATE policies SET active = 0 WHERE kind NOT IN ('duration', 'forever');
