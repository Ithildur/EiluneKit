package decoder

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

var ErrBodyTooLarge = errors.New("body too large")
var ErrInvalidJSON = errors.New("invalid json")

// DecodeJSONBody decodes a JSON request body.
// Call errors.Is(err, ErrBodyTooLarge) or errors.Is(err, ErrInvalidJSON).
// DecodeJSONBody 解码 JSON 请求体。
// 调用 errors.Is(err, ErrBodyTooLarge) 或 errors.Is(err, ErrInvalidJSON) 判断错误。
func DecodeJSONBody(r *http.Request, out interface{}) error {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(out); err != nil {
		return normalizeDecodeError(err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("%w: json body must contain a single value", ErrInvalidJSON)
	}
	return nil
}

func normalizeDecodeError(err error) error {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return fmt.Errorf("%w: %w", ErrBodyTooLarge, err)
	}
	return fmt.Errorf("%w: %w", ErrInvalidJSON, err)
}
