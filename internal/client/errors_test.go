package client

import (
	"errors"
	"net/http"
	"testing"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// mockServiceError implements common.ServiceError for testing.
type mockServiceError struct {
	statusCode int
	code       string
	message    string
}

func (e *mockServiceError) GetHTTPStatusCode() int { return e.statusCode }
func (e *mockServiceError) GetMessage() string     { return e.message }
func (e *mockServiceError) GetCode() string        { return e.code }
func (e *mockServiceError) GetOpcRequestID() string { return "test-request-id" }
func (e *mockServiceError) Error() string          { return e.message }

var _ common.ServiceError = (*mockServiceError)(nil)

func TestClassifyOCIError_ServiceError401(t *testing.T) {
	err := &mockServiceError{
		statusCode: http.StatusUnauthorized,
		code:       "NotAuthenticated",
		message:    "The required information to complete authentication was not provided",
	}

	ociErr := ClassifyOCIError(err, "get cluster")

	if ociErr.Type != ErrorTypeNotAuthenticated {
		t.Errorf("Expected ErrorTypeNotAuthenticated, got %v", ociErr.Type)
	}
	if ociErr.StatusCode != 401 {
		t.Errorf("Expected status code 401, got %d", ociErr.StatusCode)
	}
	if ociErr.Suggestion == "" {
		t.Error("Expected suggestion for auth error")
	}
}

func TestClassifyOCIError_ServiceError403(t *testing.T) {
	err := &mockServiceError{
		statusCode: http.StatusForbidden,
		code:       "NotAuthorized",
		message:    "You don't have permission to access this resource",
	}

	ociErr := ClassifyOCIError(err, "get cluster")

	if ociErr.Type != ErrorTypeNotAuthorized {
		t.Errorf("Expected ErrorTypeNotAuthorized, got %v", ociErr.Type)
	}
	if ociErr.StatusCode != 403 {
		t.Errorf("Expected status code 403, got %d", ociErr.StatusCode)
	}
	if ociErr.Suggestion == "" {
		t.Error("Expected suggestion for authorization error")
	}
}

func TestClassifyOCIError_ServiceError404(t *testing.T) {
	err := &mockServiceError{
		statusCode: http.StatusNotFound,
		code:       "NotFound",
		message:    "Resource not found",
	}

	ociErr := ClassifyOCIError(err, "get cluster")

	if ociErr.Type != ErrorTypeNotFound {
		t.Errorf("Expected ErrorTypeNotFound, got %v", ociErr.Type)
	}
	if ociErr.StatusCode != 404 {
		t.Errorf("Expected status code 404, got %d", ociErr.StatusCode)
	}
}

func TestClassifyOCIError_NotAuthorizedOrNotFound(t *testing.T) {
	err := &mockServiceError{
		statusCode: http.StatusNotFound,
		code:       "NotAuthorizedOrNotFound",
		message:    "Authorization failed or resource not found",
	}

	ociErr := ClassifyOCIError(err, "get cluster")

	if ociErr.Type != ErrorTypeNotAuthorizedOrNotFound {
		t.Errorf("Expected ErrorTypeNotAuthorizedOrNotFound, got %v", ociErr.Type)
	}
	if ociErr.Suggestion == "" {
		t.Error("Expected suggestion for NotAuthorizedOrNotFound error")
	}
}

func TestClassifyOCIError_429RateLimiting(t *testing.T) {
	err := &mockServiceError{
		statusCode: http.StatusTooManyRequests,
		code:       "TooManyRequests",
		message:    "Rate limit exceeded",
	}

	ociErr := ClassifyOCIError(err, "list clusters")

	if ociErr.Type != ErrorTypeTooManyRequests {
		t.Errorf("Expected ErrorTypeTooManyRequests, got %v", ociErr.Type)
	}
}

func TestClassifyOCIError_500ServiceError(t *testing.T) {
	err := &mockServiceError{
		statusCode: http.StatusInternalServerError,
		code:       "InternalError",
		message:    "Internal server error",
	}

	ociErr := ClassifyOCIError(err, "create session")

	if ociErr.Type != ErrorTypeServiceError {
		t.Errorf("Expected ErrorTypeServiceError, got %v", ociErr.Type)
	}
}

func TestClassifyOCIError_WrappedAuthError(t *testing.T) {
	// Test error messages that contain auth-related strings
	tests := []struct {
		name    string
		errMsg  string
		expType OCIErrorType
	}{
		{"401 in message", "failed with status 401", ErrorTypeNotAuthenticated},
		{"not authenticated", "user not authenticated", ErrorTypeNotAuthenticated},
		{"invalid signature", "invalid signature in request", ErrorTypeNotAuthenticated},
		{"403 in message", "failed with status 403", ErrorTypeNotAuthorized},
		{"permission denied", "permission denied for this resource", ErrorTypeNotAuthorized},
		{"not authorized", "user not authorized", ErrorTypeNotAuthorized},
		{"NotAuthorizedOrNotFound", "NotAuthorizedOrNotFound error occurred", ErrorTypeNotAuthorizedOrNotFound},
		{"authorization failed", "authorization failed or not found", ErrorTypeNotAuthorizedOrNotFound},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New(tc.errMsg)
			ociErr := ClassifyOCIError(err, "test operation")

			if ociErr.Type != tc.expType {
				t.Errorf("Expected %v for '%s', got %v", tc.expType, tc.errMsg, ociErr.Type)
			}
		})
	}
}

func TestClassifyOCIError_NetworkErrors(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		expType OCIErrorType
	}{
		{"timeout", "context deadline exceeded", ErrorTypeTimeout},
		{"connection refused", "dial tcp: connection refused", ErrorTypeNetwork},
		{"no such host", "no such host: api.region.oraclecloud.com", ErrorTypeNetwork},
		{"network unreachable", "network is unreachable", ErrorTypeNetwork},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := errors.New(tc.errMsg)
			ociErr := ClassifyOCIError(err, "test operation")

			if ociErr.Type != tc.expType {
				t.Errorf("Expected %v for '%s', got %v", tc.expType, tc.errMsg, ociErr.Type)
			}
		})
	}
}

func TestClassifyOCIError_UnknownError(t *testing.T) {
	err := errors.New("some random error")
	ociErr := ClassifyOCIError(err, "test operation")

	if ociErr.Type != ErrorTypeUnknown {
		t.Errorf("Expected ErrorTypeUnknown, got %v", ociErr.Type)
	}
}

func TestClassifyOCIError_NilError(t *testing.T) {
	ociErr := ClassifyOCIError(nil, "test operation")

	if ociErr != nil {
		t.Error("Expected nil for nil error input")
	}
}

func TestIsAuthError(t *testing.T) {
	authErr := &mockServiceError{statusCode: 401, code: "NotAuthenticated", message: "auth failed"}
	if !IsAuthError(authErr) {
		t.Error("Expected IsAuthError to return true for 401 error")
	}

	otherErr := errors.New("some other error")
	if IsAuthError(otherErr) {
		t.Error("Expected IsAuthError to return false for non-auth error")
	}
}

func TestIsAuthorizationError(t *testing.T) {
	authzErr := &mockServiceError{statusCode: 403, code: "NotAuthorized", message: "not authorized"}
	if !IsAuthorizationError(authzErr) {
		t.Error("Expected IsAuthorizationError to return true for 403 error")
	}

	notFoundOrAuthErr := &mockServiceError{statusCode: 404, code: "NotAuthorizedOrNotFound", message: "not found or not authorized"}
	if !IsAuthorizationError(notFoundOrAuthErr) {
		t.Error("Expected IsAuthorizationError to return true for NotAuthorizedOrNotFound error")
	}
}

func TestIsNotFoundError(t *testing.T) {
	notFoundErr := &mockServiceError{statusCode: 404, code: "NotFound", message: "resource not found"}
	if !IsNotFoundError(notFoundErr) {
		t.Error("Expected IsNotFoundError to return true for 404 error")
	}
}

func TestOCIError_Error(t *testing.T) {
	ociErr := &OCIError{
		Type:       ErrorTypeNotAuthorized,
		Operation:  "get cluster",
		Message:    "not authorized",
		Suggestion: "check your IAM policies",
	}

	errStr := ociErr.Error()
	if errStr == "" {
		t.Error("Expected non-empty error string")
	}
	if !containsAll(errStr, "get cluster", "not authorized", "check your IAM policies") {
		t.Errorf("Error string missing expected content: %s", errStr)
	}
}

func TestOCIError_ErrorWithoutSuggestion(t *testing.T) {
	ociErr := &OCIError{
		Type:      ErrorTypeUnknown,
		Operation: "test op",
		Message:   "something failed",
	}

	errStr := ociErr.Error()
	if errStr != "test op: something failed" {
		t.Errorf("Unexpected error string: %s", errStr)
	}
}

func TestOCIError_Unwrap(t *testing.T) {
	underlying := errors.New("underlying error")
	ociErr := &OCIError{
		Type:       ErrorTypeUnknown,
		Operation:  "test",
		Message:    "wrapped",
		Underlying: underlying,
	}

	if ociErr.Unwrap() != underlying {
		t.Error("Unwrap should return the underlying error")
	}
}

func TestWrapOCIError(t *testing.T) {
	err := &mockServiceError{statusCode: 401, code: "NotAuthenticated", message: "auth failed"}
	wrapped := WrapOCIError(err, "test operation")

	if wrapped == nil {
		t.Error("WrapOCIError should not return nil for non-nil error")
	}

	ociErr, ok := wrapped.(*OCIError)
	if !ok {
		t.Error("WrapOCIError should return an *OCIError")
	}

	if ociErr.Operation != "test operation" {
		t.Errorf("Expected operation 'test operation', got '%s'", ociErr.Operation)
	}
}

func TestWrapOCIError_Nil(t *testing.T) {
	wrapped := WrapOCIError(nil, "test operation")
	if wrapped != nil {
		t.Error("WrapOCIError should return nil for nil error")
	}
}

func TestGetAuthenticationSuggestion_SignatureMismatch(t *testing.T) {
	suggestion := getAuthenticationSuggestion("SignatureDoesNotMatch")
	if suggestion == "" {
		t.Error("Expected non-empty suggestion")
	}
	if !containsAll(suggestion, "private key", "fingerprint") {
		t.Errorf("Suggestion should mention key/fingerprint issues: %s", suggestion)
	}
}

func TestGetAuthorizationSuggestion_Operations(t *testing.T) {
	tests := []struct {
		operation string
		contains  []string
	}{
		{"get cluster", []string{"clusters"}},
		{"GetCluster", []string{"clusters"}},
		{"list bastions", []string{"bastion"}},
		{"ListBastions", []string{"bastion"}},
		{"list compartments", []string{"compartments"}},
		{"other operation", []string{"administrator"}},
	}

	for _, tc := range tests {
		t.Run(tc.operation, func(t *testing.T) {
			suggestion := getAuthorizationSuggestion(tc.operation)
			if !containsAll(suggestion, tc.contains...) {
				t.Errorf("Suggestion for '%s' should contain %v: %s", tc.operation, tc.contains, suggestion)
			}
		})
	}
}

// containsAll checks if s contains all the given substrings.
func containsAll(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if !contains(s, sub) {
			return false
		}
	}
	return true
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && containsSubstring(s, substr)))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
