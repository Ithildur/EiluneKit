// Package contextutil provides context helpers.
// Package contextutil 提供 context 辅助函数。
package contextutil

import (
	"context"
	"time"
)

const nilContextMessage = "contextutil: nil context"

// Require returns ctx or panics on nil.
// Require 返回 ctx；ctx 为 nil 时 panic。
func Require(ctx context.Context) context.Context {
	if ctx == nil {
		panic(nilContextMessage)
	}
	return ctx
}

// WithTimeout calls fn synchronously with context.WithTimeout(parent, d) and cancels the context when fn finishes.
// Timeout handling depends on fn observing the context.
// WithTimeout 使用 context.WithTimeout(parent, d) 同步调用 fn，并在 fn 结束时取消 context。
// 超时处理依赖 fn 响应 context。
func WithTimeout[T any](parent context.Context, d time.Duration, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(Require(parent), d)
	defer cancel()
	return fn(ctx)
}
