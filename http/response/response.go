// Package response provides JSON response helpers.
// Package response 提供 JSON 响应辅助函数。
package response

import (
	"bytes"
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standard JSON error payload.
// ErrorResponse 是标准 JSON 错误响应体。
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteJSON writes v as JSON with status.
// Call WriteJSON(w, status, value).
// WriteJSON 以 JSON 写入 value 和 status。
// 调用 WriteJSON(w, status, value)。
//
// Example / 示例:
//
//	response.WriteJSON(w, http.StatusOK, map[string]any{"ok": true})
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	if err := enc.Encode(v); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"code":"internal_error","message":"internal server error"}`))
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// WriteJSONError writes ErrorResponse as JSON.
// Call WriteJSONError(w, status, code, message).
// WriteJSONError 以 JSON 写入 ErrorResponse。
// 调用 WriteJSONError(w, status, code, message)。
//
// Example / 示例:
//
//	response.WriteJSONError(w, http.StatusBadRequest, "invalid_json", "invalid json")
func WriteJSONError(w http.ResponseWriter, status int, code, msg string) {
	WriteJSON(w, status, ErrorResponse{Code: code, Message: msg})
}
