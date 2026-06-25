// Package httpx holds small HTTP helpers shared by every domain handler:
// a consistent JSON envelope, a typed API-error model, and request decoding.
package httpx

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
)

// APIError is a client-safe error with an HTTP status, a stable machine code and
// a human message. Handlers return it to control the response; anything else is
// treated as an unexpected 500.
type APIError struct {
	Status  int    `json:"-"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string { return e.Message }

// NewError builds an APIError.
func NewError(status int, code, message string) *APIError {
	return &APIError{Status: status, Code: code, Message: message}
}

// Common constructors.
func ErrBadRequest(msg string) *APIError { return NewError(http.StatusBadRequest, "bad_request", msg) }
func ErrValidation(msg string) *APIError {
	return NewError(http.StatusUnprocessableEntity, "validation_failed", msg)
}
func ErrUnauthorized(msg string) *APIError {
	return NewError(http.StatusUnauthorized, "unauthorized", msg)
}
func ErrForbidden(msg string) *APIError { return NewError(http.StatusForbidden, "forbidden", msg) }
func ErrNotFound(msg string) *APIError  { return NewError(http.StatusNotFound, "not_found", msg) }
func ErrConflict(msg string) *APIError  { return NewError(http.StatusConflict, "conflict", msg) }

type errorEnvelope struct {
	Error APIError `json:"error"`
}

// JSON writes v as a JSON response with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// Response is already partially written; nothing actionable left to do.
		return
	}
}

// NoContent writes a 204.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// Error writes err as a JSON error envelope. Known *APIError values keep their
// status/code; everything else collapses to a generic 500 (callers should log
// the underlying error before calling Error).
func Error(w http.ResponseWriter, err error) {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		JSON(w, apiErr.Status, errorEnvelope{Error: *apiErr})
		return
	}
	JSON(w, http.StatusInternalServerError, errorEnvelope{
		Error: APIError{Code: "internal_error", Message: "something went wrong"},
	})
}

// WriteError logs unexpected (non-*APIError) errors before responding, then
// writes the JSON error envelope. Domain handlers should use this.
func WriteError(w http.ResponseWriter, log *slog.Logger, err error) {
	var apiErr *APIError
	if !errors.As(err, &apiErr) && log != nil {
		log.Error("request failed", "err", err)
	}
	Error(w, err)
}

const maxBodyBytes = 1 << 20 // 1 MiB

// Decode reads a JSON request body into dst, rejecting unknown fields and bodies
// larger than 1 MiB. It returns an *APIError suitable for Error.
func Decode(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return ErrBadRequest("request body is required")
		}
		return ErrBadRequest("invalid JSON: " + err.Error())
	}
	if dec.More() {
		return ErrBadRequest("request body must contain a single JSON object")
	}
	return nil
}
