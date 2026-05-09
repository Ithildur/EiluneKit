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

// WithTimeout runs fn with context.WithTimeout(parent, d).
// WithTimeout 使用 context.WithTimeout(parent, d) 运行 fn。
//
// Example / 示例:
//
//	result, err := contextutil.WithTimeout(ctx, 5*time.Second, func(ctx context.Context) (T, error) { ... })
func WithTimeout[T any](parent context.Context, d time.Duration, fn func(context.Context) (T, error)) (T, error) {
	ctx, cancel := context.WithTimeout(Require(parent), d)
	defer cancel()
	return fn(ctx)
}
