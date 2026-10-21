// Package captcha verifies CAPTCHA tokens with external services.
// Package captcha 通过外部服务验证 CAPTCHA token。
package captcha

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	// ErrRequired means no token was supplied.
	// ErrRequired 表示未提供 token。
	ErrRequired = errors.New("captcha: token required")
	// ErrRejected means the token or its verification policy was rejected.
	// ErrRejected 表示 token 或其验证策略未通过。
	ErrRejected = errors.New("captcha: verification rejected")
	// ErrMisconfigured means local parameters or provider credentials are invalid.
	// ErrMisconfigured 表示本地参数或服务商凭据无效。
	ErrMisconfigured = errors.New("captcha: misconfigured")
	// ErrUnavailable means verification could not be completed.
	// ErrUnavailable 表示无法完成验证。
	ErrUnavailable = errors.New("captcha: verification unavailable")
)

// Options controls verification requests. Constructors perform no network I/O.
// Options 控制验证请求。构造函数不进行网络访问。
type Options struct {
	// HTTPClient is copied; its transport remains shared and owned by the caller.
	// Nil uses http.DefaultClient. Redirects are never followed.
	// HTTPClient 会被复制；transport 仍由调用方持有并共享。
	// Nil 使用 http.DefaultClient。不会跟随重定向。
	HTTPClient *http.Client
	// Timeout bounds the entire verification. Zero defaults to five seconds.
	// Timeout 限制整个验证耗时。零值默认为五秒。
	Timeout time.Duration
}

// Client verifies tokens concurrently without caching or automatic retries.
// A nil or zero Client returns ErrMisconfigured from Verify.
// Client 支持并发验证，不缓存结果或自动重试。
// Nil 或零值 Client 的 Verify 返回 ErrMisconfigured。
type Client struct {
	httpClient *http.Client
	endpoint   string
	secret     string
	timeout    time.Duration
	cap        bool
	hostname   string
	action     string
	minScore   *float64
}

func newClient(endpoint, secret string, opts Options) (*Client, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, fmt.Errorf("%w: secret is required", ErrMisconfigured)
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || (u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, fmt.Errorf("%w: endpoint must be an absolute HTTP(S) URL without credentials, query or fragment", ErrMisconfigured)
	}
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("%w: timeout must not be negative", ErrMisconfigured)
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Second
	}
	httpClient := http.DefaultClient
	if opts.HTTPClient != nil {
		httpClient = opts.HTTPClient
	}
	owned := *httpClient
	// Redirects could forward the secret to a different endpoint.
	// 重定向可能将 secret 转发至其他端点。
	owned.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{httpClient: &owned, endpoint: endpoint, secret: secret, timeout: opts.Timeout}, nil
}

// Verify consumes token and returns nil only when all verification checks pass.
// Errors match the package sentinels with errors.Is; cancellation also matches
// context.Canceled or context.DeadlineExceeded. Error text excludes credentials,
// tokens, raw provider responses and transport error text.
// Verify 消费 token，仅在全部验证通过时返回 nil。
// 错误可通过 errors.Is 匹配包内哨兵；取消还可匹配 context.Canceled 或
// context.DeadlineExceeded。错误文本不包含凭据、token、上游原始响应或传输错误文本。
func (c *Client) Verify(ctx context.Context, token string) error {
	if c == nil || c.httpClient == nil {
		return ErrMisconfigured
	}
	if strings.TrimSpace(token) == "" {
		return ErrRequired
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	result, err := c.verify(ctx, token)
	if err != nil {
		return err
	}
	if c.hostname != "" && result.Hostname != c.hostname {
		return fmt.Errorf("%w: hostname mismatch", ErrRejected)
	}
	if c.action != "" && result.Action != c.action {
		return fmt.Errorf("%w: action mismatch", ErrRejected)
	}
	if c.minScore != nil {
		if result.Score == nil || !(*result.Score >= 0 && *result.Score <= 1) {
			return fmt.Errorf("%w: missing or invalid score", ErrUnavailable)
		}
		if *result.Score < *c.minScore {
			return fmt.Errorf("%w: score below threshold", ErrRejected)
		}
	}
	return nil
}
