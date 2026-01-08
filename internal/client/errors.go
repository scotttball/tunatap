package client

import (
	"errors"
	"fmt"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
)

// OCIErrorType represents the type of OCI API error.
type OCIErrorType int

const (
	// ErrorTypeUnknown is an unclassified error.
	ErrorTypeUnknown OCIErrorType = iota
	// ErrorTypeNotAuthenticated indicates authentication failure (401).
	ErrorTypeNotAuthenticated
	// ErrorTypeNotAuthorized indicates authorization failure (403).
	ErrorTypeNotAuthorized
	// ErrorTypeNotFound indicates the resource was not found (404).
	ErrorTypeNotFound
	// ErrorTypeNotAuthorizedOrNotFound indicates either 403 or 404 (OCI returns this for security).
	ErrorTypeNotAuthorizedOrNotFound
	// ErrorTypeTooManyRequests indicates rate limiting (429).
	ErrorTypeTooManyRequests
	// ErrorTypeServiceError indicates a general service error (5xx).
	ErrorTypeServiceError
	// ErrorTypeTimeout indicates a timeout error.
	ErrorTypeTimeout
	// ErrorTypeNetwork indicates a network connectivity error.
	ErrorTypeNetwork
)

// OCIError wraps an OCI error with additional context.
type OCIError struct {
	Type       OCIErrorType
	StatusCode int
	Code       string
	Message    string
	Operation  string
	Suggestion string
	Underlying error
}

func (e *OCIError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("%s: %s\n\nSuggestion: %s", e.Operation, e.Message, e.Suggestion)
	}
	return fmt.Sprintf("%s: %s", e.Operation, e.Message)
}

func (e *OCIError) Unwrap() error {
	return e.Underlying
}

// ClassifyOCIError examines an error and returns a classified OCIError with helpful suggestions.
func ClassifyOCIError(err error, operation string) *OCIError {
	if err == nil {
		return nil
	}

	ociErr := &OCIError{
		Type:       ErrorTypeUnknown,
		Operation:  operation,
		Message:    err.Error(),
		Underlying: err,
	}

	// Check for OCI SDK service error
	var serviceErr common.ServiceError
	if errors.As(err, &serviceErr) {
		ociErr.StatusCode = serviceErr.GetHTTPStatusCode()
		ociErr.Code = serviceErr.GetCode()
		ociErr.Message = serviceErr.GetMessage()

		switch ociErr.StatusCode {
		case 401:
			ociErr.Type = ErrorTypeNotAuthenticated
			ociErr.Suggestion = getAuthenticationSuggestion(ociErr.Code)
		case 403:
			ociErr.Type = ErrorTypeNotAuthorized
			ociErr.Suggestion = getAuthorizationSuggestion(operation)
		case 404:
			ociErr.Type = ErrorTypeNotFound
			ociErr.Suggestion = getNotFoundSuggestion(operation)
		case 429:
			ociErr.Type = ErrorTypeTooManyRequests
			ociErr.Suggestion = "Too many requests. Wait a moment and try again."
		default:
			if ociErr.StatusCode >= 500 {
				ociErr.Type = ErrorTypeServiceError
				ociErr.Suggestion = "OCI service error. Check OCI status page and try again later."
			}
		}

		// OCI often returns NotAuthorizedOrNotFound for security reasons
		if ociErr.Code == "NotAuthorizedOrNotFound" {
			ociErr.Type = ErrorTypeNotAuthorizedOrNotFound
			ociErr.Suggestion = getNotAuthorizedOrNotFoundSuggestion(operation)
		}

		return ociErr
	}

	// Check for common error patterns in wrapped errors
	errStr := strings.ToLower(err.Error())

	if strings.Contains(errStr, "notauthorizedornotfound") ||
		strings.Contains(errStr, "authorization failed") {
		ociErr.Type = ErrorTypeNotAuthorizedOrNotFound
		ociErr.Suggestion = getNotAuthorizedOrNotFoundSuggestion(operation)
		return ociErr
	}

	if strings.Contains(errStr, "401") ||
		strings.Contains(errStr, "not authenticated") ||
		strings.Contains(errStr, "authentication failed") ||
		strings.Contains(errStr, "invalid signature") {
		ociErr.Type = ErrorTypeNotAuthenticated
		ociErr.Suggestion = getAuthenticationSuggestion("")
		return ociErr
	}

	if strings.Contains(errStr, "403") ||
		strings.Contains(errStr, "not authorized") ||
		strings.Contains(errStr, "permission denied") {
		ociErr.Type = ErrorTypeNotAuthorized
		ociErr.Suggestion = getAuthorizationSuggestion(operation)
		return ociErr
	}

	if strings.Contains(errStr, "404") || strings.Contains(errStr, "not found") {
		ociErr.Type = ErrorTypeNotFound
		ociErr.Suggestion = getNotFoundSuggestion(operation)
		return ociErr
	}

	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline exceeded") {
		ociErr.Type = ErrorTypeTimeout
		ociErr.Suggestion = "Request timed out. Check your network connection and try again."
		return ociErr
	}

	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "network is unreachable") {
		ociErr.Type = ErrorTypeNetwork
		ociErr.Suggestion = "Network error. Check your internet connection and proxy settings."
		return ociErr
	}

	return ociErr
}

func getAuthenticationSuggestion(code string) string {
	suggestions := []string{
		"Authentication failed. Please check:",
		"  1. Your OCI credentials are correctly configured in ~/.oci/config",
		"  2. The API key has not expired or been revoked",
		"  3. Your system clock is synchronized (time skew can cause auth failures)",
		"",
		"If using SSO/session auth, try: oci session authenticate",
		"If using a different profile, use: --oci-profile <profile-name>",
	}

	if code == "SignatureDoesNotMatch" {
		suggestions = append([]string{
			"Signature mismatch error. This usually means:",
			"  - The private key doesn't match the public key in OCI",
			"  - The API key fingerprint is incorrect",
			"",
		}, suggestions...)
	}

	return strings.Join(suggestions, "\n")
}

func getAuthorizationSuggestion(operation string) string {
	suggestions := []string{
		"You don't have permission for this operation.",
		"",
		"Required IAM policies depend on the operation:",
	}

	switch {
	case strings.Contains(operation, "cluster") || strings.Contains(operation, "Cluster"):
		suggestions = append(suggestions,
			"  Allow group <group-name> to read clusters in compartment <compartment-name>",
			"  Allow group <group-name> to use clusters in compartment <compartment-name>")
	case strings.Contains(operation, "bastion") || strings.Contains(operation, "Bastion"):
		suggestions = append(suggestions,
			"  Allow group <group-name> to read bastion-session in compartment <compartment-name>",
			"  Allow group <group-name> to manage bastion-session in compartment <compartment-name>")
	case strings.Contains(operation, "compartment") || strings.Contains(operation, "Compartment"):
		suggestions = append(suggestions,
			"  Allow group <group-name> to inspect compartments in tenancy")
	default:
		suggestions = append(suggestions,
			"  Contact your OCI administrator to verify your group policies.")
	}

	suggestions = append(suggestions,
		"",
		"See: https://docs.oracle.com/en-us/iaas/Content/Identity/Concepts/policygetstarted.htm")

	return strings.Join(suggestions, "\n")
}

func getNotFoundSuggestion(operation string) string {
	suggestions := []string{
		"The requested resource was not found.",
		"",
		"Please verify:",
		"  1. The OCID is correct and the resource exists",
		"  2. You're using the correct region (--region flag)",
		"  3. The resource hasn't been deleted",
	}

	if strings.Contains(operation, "cluster") || strings.Contains(operation, "Cluster") {
		suggestions = append(suggestions,
			"",
			"To find cluster OCIDs, use: tunatap list",
			"Or check the OCI Console: https://cloud.oracle.com/containers/clusters")
	}

	return strings.Join(suggestions, "\n")
}

func getNotAuthorizedOrNotFoundSuggestion(operation string) string {
	suggestions := []string{
		"Resource not accessible (either it doesn't exist or you lack permissions).",
		"",
		"OCI returns this error for security - it won't reveal whether the resource exists.",
		"",
		"Please verify:",
		"  1. The OCID/name is correct",
		"  2. You have the necessary IAM policies",
		"  3. You're using the correct region",
	}

	switch {
	case strings.Contains(operation, "cluster") || strings.Contains(operation, "Cluster"):
		suggestions = append(suggestions,
			"",
			"Required policy for cluster access:",
			"  Allow group <group-name> to read clusters in compartment <compartment-name>")
	case strings.Contains(operation, "bastion") || strings.Contains(operation, "Bastion"):
		suggestions = append(suggestions,
			"",
			"Required policies for bastion access:",
			"  Allow group <group-name> to read bastions in compartment <compartment-name>",
			"  Allow group <group-name> to manage bastion-session in compartment <compartment-name>")
	}

	return strings.Join(suggestions, "\n")
}

// IsAuthError returns true if the error is an authentication error.
func IsAuthError(err error) bool {
	ociErr := ClassifyOCIError(err, "")
	return ociErr.Type == ErrorTypeNotAuthenticated
}

// IsAuthorizationError returns true if the error is an authorization error.
func IsAuthorizationError(err error) bool {
	ociErr := ClassifyOCIError(err, "")
	return ociErr.Type == ErrorTypeNotAuthorized || ociErr.Type == ErrorTypeNotAuthorizedOrNotFound
}

// IsNotFoundError returns true if the error is a not-found error.
func IsNotFoundError(err error) bool {
	ociErr := ClassifyOCIError(err, "")
	return ociErr.Type == ErrorTypeNotFound
}

// WrapOCIError wraps an OCI error with classification and context.
// Use this to provide better error messages to users.
func WrapOCIError(err error, operation string) error {
	if err == nil {
		return nil
	}
	return ClassifyOCIError(err, operation)
}
