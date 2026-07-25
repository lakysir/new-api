package limiter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Reserve 的核心不变式：DB 计数 + 有效预留数不得超过上限。
func TestConcurrencyReserverEnforcesLimit(t *testing.T) {
	r := NewConcurrencyReserver(nil)
	ctx := context.Background()
	const key = "concurrency:reserve:1:sora-2"

	allowed, used, err := r.Reserve(ctx, key, "req-1", 1, 3, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 2, used)

	allowed, used, err = r.Reserve(ctx, key, "req-2", 1, 3, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 3, used)

	// DB 计数 1 + 预留 2 == 上限 3，第三个请求必须被拒。
	allowed, used, err = r.Reserve(ctx, key, "req-3", 1, 3, time.Minute)
	require.NoError(t, err)
	assert.False(t, allowed)
	assert.Equal(t, 3, used)
}

// 释放预留位后名额应立即可用。
func TestConcurrencyReserverReleaseFreesSlot(t *testing.T) {
	r := NewConcurrencyReserver(nil)
	ctx := context.Background()
	const key = "concurrency:reserve:2:sora-2"

	allowed, _, err := r.Reserve(ctx, key, "req-1", 0, 1, time.Minute)
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, _, err = r.Reserve(ctx, key, "req-2", 0, 1, time.Minute)
	require.NoError(t, err)
	require.False(t, allowed)

	r.Release(ctx, key, "req-1")

	allowed, used, err := r.Reserve(ctx, key, "req-2", 0, 1, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 1, used)
}

// 预留位过期后自动回收，避免 Release 未执行时槽位永久泄漏。
func TestConcurrencyReserverExpiresStaleSlot(t *testing.T) {
	r := NewConcurrencyReserver(nil)
	ctx := context.Background()
	const key = "concurrency:reserve:3:sora-2"

	allowed, _, err := r.Reserve(ctx, key, "leaked", 0, 1, 10*time.Millisecond)
	require.NoError(t, err)
	require.True(t, allowed)

	time.Sleep(20 * time.Millisecond)

	allowed, used, err := r.Reserve(ctx, key, "req-2", 0, 1, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 1, used)
}

// 不同模型互不影响：并发按 (用户, 模型) 独立计算。
func TestConcurrencyReserverScopedPerKey(t *testing.T) {
	r := NewConcurrencyReserver(nil)
	ctx := context.Background()

	allowed, _, err := r.Reserve(ctx, "concurrency:reserve:4:sora-2", "req-1", 0, 1, time.Minute)
	require.NoError(t, err)
	require.True(t, allowed)

	allowed, used, err := r.Reserve(ctx, "concurrency:reserve:4:kling-v1", "req-1", 0, 1, time.Minute)
	require.NoError(t, err)
	assert.True(t, allowed)
	assert.Equal(t, 1, used)
}
