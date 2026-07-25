-- 异步任务并发预留位
-- KEYS[1]: 预留位集合 key，形如 concurrency:reserve:{userId}:{model}
-- ARGV[1]: 预留位成员标识（请求 ID）
-- ARGV[2]: 预留位存活秒数（TTL）
-- ARGV[3]: 并发上限
-- ARGV[4]: 数据库中已存在的进行中任务数
--
-- 返回 {allowed, used}：
--   allowed = 1 表示成功占位，0 表示并发已满
--   used    = 判定时的实际占用数（DB 计数 + 有效预留数）

local key = KEYS[1]
local member = ARGV[1]
local ttl = tonumber(ARGV[2])
local limit = tonumber(ARGV[3])
local db_count = tonumber(ARGV[4])

local now = tonumber(redis.call('TIME')[1])

-- 清理已过期的预留位（进程崩溃等异常场景的兜底）
redis.call('ZREMRANGEBYSCORE', key, '-inf', now)

local reserved = tonumber(redis.call('ZCARD', key))
local used = db_count + reserved

if used >= limit then
    return {0, used}
end

redis.call('ZADD', key, now + ttl, member)
redis.call('EXPIRE', key, ttl + 60)

return {1, used + 1}
