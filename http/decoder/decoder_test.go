package decoder

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyErrorContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		body    string
		limit   int64
		wantErr error
	}{
		{
			name:    "invalid_json",
			body:    `{"name":`,
			wantErr: ErrInvalidJSON,
		},
		{
			name:    "too_large",
			body:    `{"name":"kit"}`,
			limit:   4,
			wantErr: ErrBodyTooLarge,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			if tt.limit > 0 {
				req.Body = http.MaxBytesReader(httptest.NewRecorder(), req.Body, tt.limit)
			}

			var out map[string]any
			err := DecodeJSONBody(req, &out)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("DecodeJSONBody error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestDecodeJSONBodyWithOptionsRejectsUnknownFields(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kit","extra":true}`))

	var out payload
	err := DecodeJSONBodyWithOptions(req, &out, JSONOptions{DisallowUnknownFields: true})
	if !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("DecodeJSONBodyWithOptions error = %v, want %v", err, ErrInvalidJSON)
	}
}

func TestDecodeJSONBodyAllowsUnknownFieldsByDefault(t *testing.T) {
	t.Parallel()

	type payload struct {
		Name string `json:"name"`
	}
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"kit","extra":true}`))

	var out payload
	if err := DecodeJSONBody(req, &out); err != nil {
		t.Fatalf("DecodeJSONBody error = %v", err)
	}
	if out.Name != "kit" {
		t.Fatalf("expected name kit, got %q", out.Name)
	}
}
