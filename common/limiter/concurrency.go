package limiter

import (
	"context"
	_ "embed"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

//go:embed lua/concurrency_reserve.lua
var concurrencyReserveScript string

// reserveScript 由 go-redis 管理 EVALSHA/EVAL 回退，无需手工处理 NOSCRIPT。
var reserveScript = redis.NewScript(concurrencyReserveScript)

// ConcurrencyReserver 为异步任务提供「提交中」的短期预留位。
//
// 异步任务的真正并发数以数据库中进行中的任务为准，但从并发判定到任务落库之间
// 存在一个窗口，同时到达的请求会读到相同的数据库计数而双双通过。预留位覆盖这个
// 窗口：判定值 = 数据库计数 + 有效预留数。
//
// 预留位自带过期时间，即使进程崩溃或 Release 未被执行，也会在 TTL 后自动失效，
// 不会出现槽位泄漏导致用户被永久锁死的情况。
type ConcurrencyReserver struct {
	rdb *redis.Client

	// mu 保护 memory，仅在未启用 Redis 时使用。
	mu     sync.Mutex
	memory map[string]map[string]time.Time
}

// NewConcurrencyReserver 创建预留位管理器。rdb 为 nil 时退化为进程内实现，
// 此时多实例部署下每个实例独立计数，存在短时超发。
func NewConcurrencyReserver(rdb *redis.Client) *ConcurrencyReserver {
	return &ConcurrencyReserver{
		rdb:    rdb,
		memory: make(map[string]map[string]time.Time),
	}
}

// Reserve 尝试占用一个并发位。dbCount 是调用方从数据库查到的进行中任务数，
// limit 是并发上限（调用方保证 > 0）。
// 返回 allowed 表示是否允许提交，used 表示判定时的实际占用数（用于错误提示）。
func (r *ConcurrencyReserver) Reserve(ctx context.Context, key string, member string, dbCount int64, limit int, ttl time.Duration) (allowed bool, used int, err error) {
	if r.rdb == nil {
		return r.reserveInMemory(key, member, dbCount, limit, ttl)
	}

	result, err := reserveScript.Run(ctx, r.rdb, []string{key}, member, int64(ttl.Seconds()), limit, dbCount).Slice()
	if err != nil {
		return false, 0, fmt.Errorf("reserve concurrency slot failed: %w", err)
	}
	if len(result) != 2 {
		return false, 0, fmt.Errorf("reserve concurrency slot returned unexpected result: %v", result)
	}

	allowedVal, ok := result[0].(int64)
	if !ok {
		return false, 0, fmt.Errorf("reserve concurrency slot returned unexpected allowed value: %v", result[0])
	}
	usedVal, ok := result[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("reserve concurrency slot returned unexpected used value: %v", result[1])
	}
	return allowedVal == 1, int(usedVal), nil
}

// Release 释放预留位。任务已落库（数据库计数已能反映它）或提交失败时调用。
func (r *ConcurrencyReserver) Release(ctx context.Context, key string, member string) {
	if r.rdb == nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if members := r.memory[key]; members != nil {
			delete(members, member)
			if len(members) == 0 {
				delete(r.memory, key)
			}
		}
		return
	}
	r.rdb.ZRem(ctx, key, member)
}

func (r *ConcurrencyReserver) reserveInMemory(key string, member string, dbCount int64, limit int, ttl time.Duration) (bool, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	members := r.memory[key]
	if members == nil {
		members = make(map[string]time.Time)
		r.memory[key] = members
	}
	for existing, deadline := range members {
		if deadline.Before(now) {
			delete(members, existing)
		}
	}

	used := int(dbCount) + len(members)
	if used >= limit {
		if len(members) == 0 {
			delete(r.memory, key)
		}
		return false, used, nil
	}

	members[member] = now.Add(ttl)
	return true, used + 1, nil
}
