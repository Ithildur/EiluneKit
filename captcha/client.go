package captcha

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxResponseBytes = 64 << 10

type verificationResponse struct {
	Success    *bool    `json:"success"`
	Hostname   string   `json:"hostname"`
	Action     string   `json:"action"`
	Score      *float64 `json:"score"`
	ErrorCodes []string `json:"error-codes"`
	Error      string   `json:"error"`
}

func (c *Client) verify(ctx context.Context, token string) (verificationResponse, error) {
	var result verificationResponse
	var body io.Reader
	contentType := "application/x-www-form-urlencoded"
	if c.cap {
		payload, err := json.Marshal(struct {
			Secret   string `json:"secret"`
			Response string `json:"response"`
		}{c.secret, token})
		if err != nil {
			return result, fmt.Errorf("%w: invalid request encoding", ErrRejected)
		}
		body = bytes.NewReader(payload)
		contentType = "application/json"
	} else {
		body = strings.NewReader(url.Values{"secret": {c.secret}, "response": {token}}.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, body)
	if err != nil {
		return result, fmt.Errorf("%w: cannot construct request", ErrMisconfigured)
	}
	req.Header.Set("Content-Type", contentType)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return result, requestError(ctx)
	}
	defer resp.Body.Close()
	// Cap reports invalid tokens and credentials using non-2xx responses.
	// Cap 使用非 2xx 响应报告无效 token 和凭据。
	capFailure := c.cap && (resp.StatusCode == 400 || resp.StatusCode == 403 || resp.StatusCode == 404)
	if resp.StatusCode != http.StatusOK && !capFailure {
		return result, fmt.Errorf("%w: HTTP status %d", ErrUnavailable, resp.StatusCode)
	}
	payload, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return result, requestError(ctx)
	}
	if len(payload) > maxResponseBytes {
		return result, fmt.Errorf("%w: response too large", ErrUnavailable)
	}
	if err := json.Unmarshal(payload, &result); err != nil || result.Success == nil {
		return result, fmt.Errorf("%w: invalid response", ErrUnavailable)
	}
	if *result.Success {
		if resp.StatusCode != http.StatusOK || len(result.ErrorCodes) != 0 || result.Error != "" {
			return result, fmt.Errorf("%w: inconsistent response", ErrUnavailable)
		}
		return result, nil
	}
	if c.cap {
		switch result.Error {
		case "Invalid site key or secret":
			return result, ErrMisconfigured
		case "Missing required parameters", "Token not found", "Token expired":
			return result, ErrRejected
		case "":
			if resp.StatusCode != http.StatusOK {
				return result, fmt.Errorf("%w: unrecognized Cap response", ErrUnavailable)
			}
		default:
			return result, fmt.Errorf("%w: unrecognized Cap error", ErrUnavailable)
		}
	}
	return result, verificationError(result.ErrorCodes)
}

func verificationError(codes []string) error {
	err := ErrRejected
	for _, code := range codes {
		switch code {
		case "missing-input-secret", "invalid-input-secret":
			return ErrMisconfigured
		case "missing-input-response", "invalid-input-response", "timeout-or-duplicate":
		default:
			err = ErrUnavailable
		}
	}
	return err
}

func requestError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return fmt.Errorf("%w: request failed", ErrUnavailable)
}
