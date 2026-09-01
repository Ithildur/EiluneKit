package decoder

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"net/http"
)

var ErrBodyTooLarge = errors.New("body too large")
var ErrInvalidJSON = errors.New("invalid json")

// JSONOptions configures DecodeJSONBodyWithOptions.
// Zero value preserves DecodeJSONBody behavior.
// JSONOptions 配置 DecodeJSONBodyWithOptions。
// 零值保持 DecodeJSONBody 行为。
type JSONOptions struct {
	// DisallowUnknownFields rejects object fields that are not present in the target struct.
	// DisallowUnknownFields 拒绝目标结构体中不存在的对象字段。
	DisallowUnknownFields bool
}

// DecodeJSONBody decodes a JSON request body.
// It rejects duplicate names and invalid UTF-8 and matches struct fields case-sensitively.
// Call errors.Is(err, ErrBodyTooLarge) or errors.Is(err, ErrInvalidJSON).
// DecodeJSONBody 解码 JSON 请求体。
// 它拒绝重复字段名和无效 UTF-8，并区分大小写匹配结构体字段。
// 调用 errors.Is(err, ErrBodyTooLarge) 或 errors.Is(err, ErrInvalidJSON) 判断错误。
func DecodeJSONBody(r *http.Request, out any) error {
	return DecodeJSONBodyWithOptions(r, out, JSONOptions{})
}

// DecodeJSONBodyWithOptions decodes a JSON request body with options.
// It rejects duplicate names and invalid UTF-8 and matches struct fields case-sensitively.
// Call errors.Is(err, ErrBodyTooLarge) or errors.Is(err, ErrInvalidJSON).
// DecodeJSONBodyWithOptions 按选项解码 JSON 请求体。
// 它拒绝重复字段名和无效 UTF-8，并区分大小写匹配结构体字段。
// 调用 errors.Is(err, ErrBodyTooLarge) 或 errors.Is(err, ErrInvalidJSON) 判断错误。
func DecodeJSONBodyWithOptions(r *http.Request, out any, opts JSONOptions) error {
	var jsonOpts []json.Options
	if opts.DisallowUnknownFields {
		jsonOpts = append(jsonOpts, json.RejectUnknownMembers(true))
	}
	if err := json.UnmarshalRead(r.Body, out, jsonOpts...); err != nil {
		return normalizeDecodeError(err)
	}
	return nil
}

func normalizeDecodeError(err error) error {
	if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
		return fmt.Errorf("%w: %w", ErrBodyTooLarge, err)
	}
	return fmt.Errorf("%w: %w", ErrInvalidJSON, err)
}
