package captcha_test

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Ithildur/EiluneKit/captcha"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func localHTTPClient(t *testing.T, handler http.HandlerFunc) *http.Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		r.URL.Scheme, r.URL.Host = endpoint.Scheme, endpoint.Host
		return server.Client().Transport.RoundTrip(r)
	})}
}

func TestProviderVerification(t *testing.T) {
	const secret = "private+key&=/汉"
	const token = "site:token+&=/汉:secret"
	providers := []struct {
		name string
		path string
		new  func(captcha.Options) (*captcha.Client, error)
	}{
		{"cap", "/site/siteverify", func(opts captcha.Options) (*captcha.Client, error) {
			return captcha.NewCap("https://cap.example/site/siteverify", secret, opts)
		}},
		{"turnstile", "/turnstile/v0/siteverify", func(opts captcha.Options) (*captcha.Client, error) {
			return captcha.NewTurnstile(secret, captcha.TurnstileOptions{Options: opts, Hostname: "example.com", Action: "login"})
		}},
		{"recaptcha_v2", "/recaptcha/api/siteverify", func(opts captcha.Options) (*captcha.Client, error) {
			return captcha.NewRecaptchaV2(secret, captcha.RecaptchaV2Options{Options: opts, Hostname: "example.com"})
		}},
		{"recaptcha_v3", "/recaptcha/api/siteverify", func(opts captcha.Options) (*captcha.Client, error) {
			return captcha.NewRecaptchaV3(secret, captcha.RecaptchaV3Options{Options: opts, Hostname: "example.com", Action: "login"})
		}},
	}
	for _, provider := range providers {
		t.Run(provider.name, func(t *testing.T) {
			type responseCase struct {
				name   string
				status int
				body   string
				want   error
			}
			tests := []responseCase{
				{"success", 200, `{"success":true,"hostname":"example.com","action":"login","score":0.5,"extra":"ignored"}`, nil},
				{"rejected", 200, `{"success":false}`, captcha.ErrRejected},
				{"bad_secret", 200, `{"success":false,"error-codes":["invalid-input-secret"]}`, captcha.ErrMisconfigured},
				{"missing_secret", 200, `{"success":false,"error-codes":["missing-input-secret"]}`, captcha.ErrMisconfigured},
				{"multiple_errors", 200, `{"success":false,"error-codes":["internal-error","invalid-input-secret"]}`, captcha.ErrMisconfigured},
				{"invalid_token", 200, `{"success":false,"error-codes":["invalid-input-response"]}`, captcha.ErrRejected},
				{"expired_or_reused", 200, `{"success":false,"error-codes":["timeout-or-duplicate"]}`, captcha.ErrRejected},
				{"internal_error", 200, `{"success":false,"error-codes":["internal-error"]}`, captcha.ErrUnavailable},
				{"unknown_error", 200, `{"success":false,"error-codes":["` + secret + `"]}`, captcha.ErrUnavailable},
				{"http_failure", 503, `{"success":true}`, captcha.ErrUnavailable},
				{"redirect", 307, `{"success":true}`, captcha.ErrUnavailable},
				{"missing_success", 200, `{}`, captcha.ErrUnavailable},
				{"null", 200, `null`, captcha.ErrUnavailable},
				{"string_success", 200, `{"success":"true"}`, captcha.ErrUnavailable},
				{"malformed", 200, `{"success":true`, captcha.ErrUnavailable},
				{"trailing_json", 200, `{"success":true} {}`, captcha.ErrUnavailable},
				{"conflicting_result", 200, `{"success":true,"error-codes":["invalid-input-secret"]}`, captcha.ErrUnavailable},
				{"oversized", 200, `{"success":true,"padding":"` + strings.Repeat("x", 64<<10) + `"}`, captcha.ErrUnavailable},
			}
			if provider.name == "cap" {
				tests = append(tests,
					responseCase{"cap_secret", 403, `{"success":false,"error":"Invalid site key or secret"}`, captcha.ErrMisconfigured},
					responseCase{"cap_site", 404, `{"success":false,"error":"Invalid site key or secret"}`, captcha.ErrMisconfigured},
					responseCase{"cap_missing", 400, `{"success":false,"error":"Missing required parameters"}`, captcha.ErrRejected},
					responseCase{"cap_expired", 403, `{"success":false,"error":"Token expired"}`, captcha.ErrRejected},
					responseCase{"cap_spent", 404, `{"success":false,"error":"Token not found"}`, captcha.ErrRejected},
					responseCase{"cap_proxy_error", 404, `<html>not found</html>`, captcha.ErrUnavailable},
				)
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					var calls atomic.Int32
					httpClient := localHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						if r.Method != http.MethodPost || r.URL.Path != provider.path {
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
						}
						var gotSecret, gotToken string
						if provider.name == "cap" {
							if r.Header.Get("Content-Type") != "application/json" {
								t.Error("Cap requires JSON")
							}
							var body map[string]string
							if err := json.UnmarshalRead(r.Body, &body); err != nil {
								t.Error(err)
							}
							gotSecret, gotToken = body["secret"], body["response"]
						} else {
							if err := r.ParseForm(); err != nil {
								t.Error(err)
							}
							gotSecret, gotToken = r.PostForm.Get("secret"), r.PostForm.Get("response")
						}
						if gotSecret != secret || gotToken != token {
							t.Error("request values did not round trip")
						}
						w.Header().Set("Location", "/redirected")
						w.WriteHeader(tt.status)
						_, _ = io.WriteString(w, tt.body)
					})
					client, err := provider.new(captcha.Options{HTTPClient: httpClient})
					if err != nil {
						t.Fatal(err)
					}
					err = client.Verify(t.Context(), token)
					if !errors.Is(err, tt.want) {
						t.Fatalf("got %v, want %v", err, tt.want)
					}
					if calls.Load() != 1 {
						t.Fatalf("expected one call, got %d", calls.Load())
					}
					if err != nil && (strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), token)) {
						t.Fatal("error exposes credentials or token")
					}
				})
			}
		})
	}
}

func TestVerificationPolicy(t *testing.T) {
	for _, tt := range []struct {
		name, body string
		want       error
	}{
		{"pass", `{"success":true,"hostname":"example.com","action":"login","score":0.8}`, nil},
		{"hostname", `{"success":true,"hostname":"other.com","action":"login","score":0.8}`, captcha.ErrRejected},
		{"action", `{"success":true,"hostname":"example.com","action":"signup","score":0.8}`, captcha.ErrRejected},
		{"low_score", `{"success":true,"hostname":"example.com","action":"login","score":0.4}`, captcha.ErrRejected},
		{"missing_score", `{"success":true,"hostname":"example.com","action":"login"}`, captcha.ErrUnavailable},
		{"null_score", `{"success":true,"hostname":"example.com","action":"login","score":null}`, captcha.ErrUnavailable},
		{"invalid_score", `{"success":true,"hostname":"example.com","action":"login","score":2}`, captcha.ErrUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			httpClient := localHTTPClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, tt.body) })
			score := 0.7
			client, err := captcha.NewRecaptchaV3("secret", captcha.RecaptchaV3Options{
				HTTPClient: httpClient, Hostname: "example.com", Action: "login", MinScore: &score,
			})
			if err != nil {
				t.Fatal(err)
			}
			score = 0
			if err := client.Verify(t.Context(), "token"); !errors.Is(err, tt.want) {
				t.Fatalf("got %v, want %v", err, tt.want)
			}
		})
	}
	for _, makeClient := range []func(*http.Client) (*captcha.Client, error){
		func(h *http.Client) (*captcha.Client, error) {
			return captcha.NewTurnstile("secret", captcha.TurnstileOptions{HTTPClient: h, Hostname: "example.com", Action: "login"})
		},
		func(h *http.Client) (*captcha.Client, error) {
			return captcha.NewRecaptchaV2("secret", captcha.RecaptchaV2Options{HTTPClient: h, Hostname: "example.com"})
		},
	} {
		h := localHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.WriteString(w, `{"success":true,"hostname":"wrong.com","action":"signup"}`)
		})
		client, err := makeClient(h)
		if err != nil {
			t.Fatal(err)
		}
		if err := client.Verify(t.Context(), "token"); !errors.Is(err, captcha.ErrRejected) {
			t.Fatalf("policy accepted: %v", err)
		}
	}
}

func TestConstructionAndEmptyToken(t *testing.T) {
	for _, endpoint := range []string{"", "/siteverify", "file:///secret", "https://user:pass@example.com/siteverify", "https://example.com/?secret=private", "https://example.com/#secret"} {
		if _, err := captcha.NewCap(endpoint, "secret", captcha.Options{}); !errors.Is(err, captcha.ErrMisconfigured) {
			t.Fatalf("accepted invalid endpoint: %v", err)
		}
	}
	if _, err := captcha.NewTurnstile(" ", captcha.TurnstileOptions{}); !errors.Is(err, captcha.ErrMisconfigured) {
		t.Fatalf("empty secret: %v", err)
	}
	if _, err := captcha.NewTurnstile("secret", captcha.TurnstileOptions{Timeout: -1}); !errors.Is(err, captcha.ErrMisconfigured) {
		t.Fatalf("negative timeout: %v", err)
	}
	if _, err := captcha.NewRecaptchaV3("secret", captcha.RecaptchaV3Options{}); !errors.Is(err, captcha.ErrMisconfigured) {
		t.Fatalf("missing action: %v", err)
	}
	for _, score := range []float64{-1, 2, math.NaN(), math.Inf(1)} {
		if _, err := captcha.NewRecaptchaV3("secret", captcha.RecaptchaV3Options{Action: "login", MinScore: new(score)}); !errors.Is(err, captcha.ErrMisconfigured) {
			t.Fatalf("invalid score: %v", err)
		}
	}
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { t.Fatal("unexpected network access"); return nil, nil })}
	client, err := captcha.NewTurnstile("secret", captcha.TurnstileOptions{HTTPClient: httpClient})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Verify(t.Context(), " "); !errors.Is(err, captcha.ErrRequired) {
		t.Fatal(err)
	}
	for _, client := range []*captcha.Client{nil, {}} {
		if err := client.Verify(t.Context(), "token"); !errors.Is(err, captcha.ErrMisconfigured) {
			t.Fatal(err)
		}
	}
}

func TestTurnstileAction(t *testing.T) {
	h := localHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"success":true,"hostname":"example.com","action":"signup"}`)
	})
	client, err := captcha.NewTurnstile("secret", captcha.TurnstileOptions{HTTPClient: h, Hostname: "example.com", Action: "login"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Verify(t.Context(), "token"); !errors.Is(err, captcha.ErrRejected) {
		t.Fatalf("accepted a token for another action: %v", err)
	}
}

func TestCancellationAndNoCache(t *testing.T) {
	for _, cancelRequest := range []bool{false, true} {
		t.Run(fmt.Sprint("cancel=", cancelRequest), func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			h := localHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				if cancelRequest {
					cancel()
				}
				<-r.Context().Done()
			})
			client, err := captcha.NewTurnstile("secret", captcha.TurnstileOptions{HTTPClient: h, Timeout: 100 * time.Millisecond})
			if err != nil {
				t.Fatal(err)
			}
			err = client.Verify(ctx, "token")
			cause := context.DeadlineExceeded
			if cancelRequest {
				cause = context.Canceled
			}
			if !errors.Is(err, captcha.ErrUnavailable) || !errors.Is(err, cause) {
				t.Fatalf("unexpected cancellation error: %v", err)
			}
		})
	}
	var calls atomic.Int32
	h := localHTTPClient(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"success":true}`)
		} else {
			_, _ = io.WriteString(w, `{"success":false,"error-codes":["timeout-or-duplicate"]}`)
		}
	})
	client, err := captcha.NewTurnstile("secret", captcha.TurnstileOptions{HTTPClient: h})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Verify(t.Context(), "token"); err != nil {
		t.Fatal(err)
	}
	if err := client.Verify(t.Context(), "token"); !errors.Is(err, captcha.ErrRejected) {
		t.Fatalf("cached success: %v", err)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected two calls, got %d", calls.Load())
	}
}

func TestTransportErrorDoesNotExposeCredentials(t *testing.T) {
	h := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("private-secret private-token") })}
	client, err := captcha.NewTurnstile("private-secret", captcha.TurnstileOptions{HTTPClient: h})
	if err != nil {
		t.Fatal(err)
	}
	err = client.Verify(t.Context(), "private-token")
	if !errors.Is(err, captcha.ErrUnavailable) || strings.Contains(err.Error(), "private-") {
		t.Fatalf("unsafe error: %v", err)
	}
}
