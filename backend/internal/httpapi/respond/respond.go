package respond

import (
	"encoding/json"
	"net/http"
)

type successEnvelope struct {
	Data any `json:"data"`
	Meta any `json:"meta,omitempty"`
}

type errorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
}

// JSON writes a standardized success envelope response with HTTP status code.
func JSON(w http.ResponseWriter, status int, data any, meta any) {
	writeRaw(w, status, successEnvelope{Data: data, Meta: meta})
}

// Error writes a standardized error envelope response matching the OpenAPI contract.
func Error(w http.ResponseWriter, status int, code, message string, fields map[string]string) {
	writeRaw(w, status, errorEnvelope{
		Error: apiError{
			Code:    code,
			Message: message,
			Fields:  fields,
		},
	})
}

func writeRaw(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

