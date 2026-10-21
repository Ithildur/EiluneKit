package captcha

import (
	"fmt"
	"strings"
)

// NewCap uses the full siteverify URL, including the site key, and its secret.
// NewCap 使用包含 site key 的完整 siteverify URL 及其 secret。
func NewCap(endpoint, secret string, opts Options) (*Client, error) {
	c, err := newClient(endpoint, secret, opts)
	if err != nil {
		return nil, err
	}
	c.cap = true
	return c, nil
}

// TurnstileOptions adds exact hostname and action checks when non-empty.
// TurnstileOptions 在 hostname 和 action 非空时执行精确匹配。
type TurnstileOptions struct {
	Options
	Hostname string
	Action   string
}

// NewTurnstile constructs a Cloudflare Turnstile verifier.
// NewTurnstile 构造 Cloudflare Turnstile 验证客户端。
func NewTurnstile(secret string, opts TurnstileOptions) (*Client, error) {
	c, err := newClient("https://challenges.cloudflare.com/turnstile/v0/siteverify", secret, opts.Options)
	if err != nil {
		return nil, err
	}
	c.hostname, c.action = opts.Hostname, opts.Action
	return c, nil
}

// RecaptchaV2Options adds an exact hostname check when non-empty.
// RecaptchaV2Options 在 hostname 非空时执行精确匹配。
type RecaptchaV2Options struct {
	Options
	Hostname string
}

// NewRecaptchaV2 constructs a Google reCAPTCHA v2 siteverify client.
// NewRecaptchaV2 构造 Google reCAPTCHA v2 siteverify 客户端。
func NewRecaptchaV2(secret string, opts RecaptchaV2Options) (*Client, error) {
	c, err := newClient("https://www.google.com/recaptcha/api/siteverify", secret, opts.Options)
	if err != nil {
		return nil, err
	}
	c.hostname = opts.Hostname
	return c, nil
}

// RecaptchaV3Options requires an action and checks the returned score.
// RecaptchaV3Options 要求提供 action 并检查返回的评分。
type RecaptchaV3Options struct {
	Options
	Hostname string
	Action   string
	// MinScore is in [0, 1]. Nil defaults to 0.5; the value is copied.
	// MinScore 取值为 [0, 1]。Nil 默认为 0.5；值会被复制。
	MinScore *float64
}

// NewRecaptchaV3 constructs a Google reCAPTCHA v3 siteverify client.
// Action must be non-empty; hostname and action checks use exact matching.
// NewRecaptchaV3 构造 Google reCAPTCHA v3 siteverify 客户端。
// Action 不能为空；hostname 和 action 使用精确匹配。
func NewRecaptchaV3(secret string, opts RecaptchaV3Options) (*Client, error) {
	if strings.TrimSpace(opts.Action) == "" {
		return nil, fmt.Errorf("%w: action is required for reCAPTCHA v3", ErrMisconfigured)
	}
	score := 0.5
	if opts.MinScore != nil {
		score = *opts.MinScore
	}
	if !(score >= 0 && score <= 1) {
		return nil, fmt.Errorf("%w: minimum score must be between zero and one", ErrMisconfigured)
	}
	c, err := newClient("https://www.google.com/recaptcha/api/siteverify", secret, opts.Options)
	if err != nil {
		return nil, err
	}
	c.hostname, c.action, c.minScore = opts.Hostname, opts.Action, new(score)
	return c, nil
}
