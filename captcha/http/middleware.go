// Package captchahttp protects HTTP endpoints with CAPTCHA verification.
// Package captchahttp 为 HTTP 端点提供 CAPTCHA 验证。
package captchahttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Ithildur/EiluneKit/captcha"
	"github.com/Ithildur/EiluneKit/http/response"
)

// TokenHeader carries the token returned by the browser widget.
// TokenHeader 携带浏览器组件返回的 token。
const TokenHeader = "X-Captcha-Token"

// Options enables verification using an application-supplied function.
// Options 通过应用提供的函数启用验证。
type Options struct {
	// Enabled defaults to false; disabled protection never calls Verify.
	// Enabled 默认关闭；关闭时不会调用 Verify。
	Enabled bool
	// Verify must be non-nil when enabled, safe for concurrent calls, and return
	// nil only on success. Use a captcha.Client's Verify method or a custom verifier.
	// 启用时 Verify 不能为 nil，必须支持并发调用，且仅在成功时返回 nil。
	// 可传入 captcha.Client 的 Verify 方法或自定义验证函数。
	Verify func(context.Context, string) error
}

// New returns nil when disabled. When enabled, a missing Verify is an error.
// The middleware reads TokenHeader without consuming the request body.
// New 在关闭时返回 nil。启用但缺少 Verify 时返回错误。
// 中间件读取 TokenHeader，不消费请求体。
func New(opts Options) (func(http.Handler) http.Handler, error) {
	if !opts.Enabled {
		return nil, nil
	}
	if opts.Verify == nil {
		return nil, fmt.Errorf("%w: verification function is required", captcha.ErrMisconfigured)
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := r.Header.Get(TokenHeader)
			if strings.TrimSpace(token) == "" {
				writeError(w, captcha.ErrRequired)
				return
			}
			if err := opts.Verify(r.Context(), token); err != nil {
				writeError(w, err)
				return
			}
			next.ServeHTTP(w, r)
		})
	}, nil
}

func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, captcha.ErrMisconfigured):
		response.WriteJSONError(w, http.StatusInternalServerError, "captcha_misconfigured", "captcha is misconfigured")
	case errors.Is(err, captcha.ErrRequired):
		response.WriteJSONError(w, http.StatusBadRequest, "captcha_required", "captcha token is required")
	case errors.Is(err, captcha.ErrRejected):
		response.WriteJSONError(w, http.StatusForbidden, "captcha_rejected", "captcha verification rejected")
	default:
		response.WriteJSONError(w, http.StatusServiceUnavailable, "captcha_unavailable", "captcha verification unavailable")
	}
}
