package types

import (
	"encoding/json"
	"net/http"
)

// APIError is the canonical error envelope returned by all Cortex API endpoints.
// This replaces all bare http.Error() calls with a structured, machine-parseable format.
//
// Shape:
//
//	{
//	  "error": {
//	    "code":    "session_not_found",
//	    "message": "Session with ID 'abc-123' does not exist.",
//	    "type":    "not_found",
//	    "param":   "session_id",
//	    "details": null
//	  }
//	}
type APIError struct {
	Code    string `json:"code"`              // Machine-readable: "session_not_found", "malformed_json"
	Message string `json:"message"`           // Human-readable explanation
	Type    string `json:"type"`              // Error category (see ErrorType* constants)
	Param   string `json:"param,omitempty"`   // Which request parameter caused the error
	Details any    `json:"details,omitempty"` // Optional structured details (validation errors, etc.)
}

// APIErrorResponse wraps APIError in the top-level envelope.
type APIErrorResponse struct {
	Error APIError `json:"error"`
}

// --- Error type categories (compatible with OpenAI convention) ---

const (
	ErrorTypeInvalidRequest = "invalid_request_error"
	ErrorTypeNotFound       = "not_found"
	ErrorTypeAuthentication = "authentication_error"
	ErrorTypeRateLimit      = "rate_limit_error"
	ErrorTypeServer         = "server_error"
	ErrorTypeProvider       = "provider_error"
	ErrorTypeTimeout        = "timeout_error"
	ErrorTypeConflict       = "conflict_error"
)

// --- Machine-readable error codes ---

const (
	ErrCodeValidation       = "validation_error"
	ErrCodeMalformedJSON    = "malformed_json"
	ErrCodeSessionNotFound  = "session_not_found"
	ErrCodeModelNotFound    = "model_not_found"
	ErrCodeProviderNotFound = "provider_not_found"
	ErrCodeProviderOffline  = "provider_offline"
	ErrCodeStreamFailed     = "stream_failed"
	ErrCodeUnauthorized     = "unauthorized"
	ErrCodeRateLimited      = "rate_limited"
	ErrCodeInternalError    = "internal_error"
	ErrCodeSessionBusy      = "session_busy"
	ErrCodeUnsupportedMedia = "unsupported_media_type"
)

// WriteError writes a structured JSON error response.
// This MUST be used instead of http.Error() everywhere.
func WriteError(w http.ResponseWriter, status int, code, errType, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
			Type:    errType,
		},
	})
}

// WriteErrorWithParam writes a structured error response including the offending parameter name.
func WriteErrorWithParam(w http.ResponseWriter, status int, code, errType, message, param string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(APIErrorResponse{
		Error: APIError{
			Code:    code,
			Message: message,
			Type:    errType,
			Param:   param,
		},
	})
}

// Error implements the error interface so APIError can be used as a Go error.
func (e APIError) Error() string {
	return e.Message
}
