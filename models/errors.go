package models

import (
	"errors"
	"time"
)

// ErrorKind classifies a failure; the HTTP layer maps it to a status.
type ErrorKind string

const (
	KindValidation  ErrorKind = "VALIDATION"
	KindNotFound    ErrorKind = "NOT_FOUND"
	KindConflict    ErrorKind = "CONFLICT"
	KindForbidden   ErrorKind = "FORBIDDEN"
	KindRateLimited ErrorKind = "RATE_LIMITED"
	KindUnavailable ErrorKind = "UNAVAILABLE"
	KindInternal    ErrorKind = "INTERNAL"
)

// ResponseCode is one entry of the public response-code catalog: a unique
// numeric code, its kind and the message returned to clients. Every code
// is defined once, here, so clients can rely on it never changing meaning.
type ResponseCode struct {
	Code    int
	Kind    ErrorKind
	Message string
}

// Response-code catalog.
//
//	2000     success
//	1xxx  request, header and input validation
//	2xxx  registration
//	5xxx  server-side failures
var (
	CodeSuccess = ResponseCode{2000, "", "Success"}

	CodeServiceProviderHeaderRequired = ResponseCode{1000, KindValidation, "Service Provider Id Header is required"}
	CodeServiceProviderHeaderInvalid  = ResponseCode{1001, KindValidation, "Service Provider Id Header must be a valid UUID"}
	CodeRequestIDHeaderRequired       = ResponseCode{1002, KindValidation, "Request Id Header is required"}
	CodeRequestIDHeaderInvalid        = ResponseCode{1003, KindValidation, "Request Id Header must be 1-64 characters of letters, digits, '.', '_' or '-'"}
	CodeTimestampHeaderRequired       = ResponseCode{1004, KindValidation, "Timestamp Header is required"}
	CodeTimestampHeaderInvalid        = ResponseCode{1005, KindValidation, "Timestamp Header must be a Unix time in seconds"}
	CodeTimestampOutOfWindow          = ResponseCode{1006, KindValidation, "Timestamp Header is outside the allowed time window"}
	CodeDuplicateRequest              = ResponseCode{1007, KindConflict, "Duplicate request: this Request Id and Timestamp were already used"}
	CodeInvalidRequestBody            = ResponseCode{1008, KindValidation, "Request Body must be a valid JSON object with only the documented fields"}
	CodeRequestBodyTooLarge           = ResponseCode{1009, KindValidation, "Request Body is too large"}
	CodeRouteNotFound                 = ResponseCode{1010, KindNotFound, "Route not found"}
	CodeDeviceIDHeaderRequired        = ResponseCode{1011, KindValidation, "Device Id Header is required"}
	CodeDeviceIDHeaderInvalid         = ResponseCode{1012, KindValidation, "Device Id Header must be at most 128 characters"}
	CodeDeviceTypeHeaderRequired      = ResponseCode{1013, KindValidation, "Device Type Header is required"}
	CodeDeviceTypeHeaderInvalid       = ResponseCode{1014, KindValidation, "Device Type Header must be one of MOBILE_IOS, MOBILE_ANDROID, WEB, DESKTOP, API_CLIENT"}
	CodeDeviceHeaderTooLong           = ResponseCode{1015, KindValidation, "Device Name, OS Name, OS Version or App Version Header is too long"}
	CodePhoneNumberRequired           = ResponseCode{1100, KindValidation, "Phone Number is required"}
	CodePhoneNumberInvalid            = ResponseCode{1101, KindValidation, "Phone Number must be in international format, e.g. +923001234567"}

	CodeServiceProviderNotFound = ResponseCode{2000, KindNotFound, "Service Provider not found"}
	CodePhoneAlreadyRegistered  = ResponseCode{2001, KindConflict, "Phone Number is already registered"}
	CodeOTPResendCooldown       = ResponseCode{2002, KindRateLimited, "Please wait before requesting another code"}
	CodeOTPHourlyLimit          = ResponseCode{2003, KindRateLimited, "Too many codes requested for this Phone Number, try again later"}
	CodeOTPDeliveryFailed       = ResponseCode{2004, KindUnavailable, "Could not send the verification code, please try again later"}

	CodeInternal           = ResponseCode{5000, KindInternal, "Internal server error"}
	CodeServiceUnavailable = ResponseCode{5001, KindUnavailable, "Service temporarily unavailable, please try again later"}
)

// AppError is a client-safe error returned by services: its ResponseCode,
// Data and RetryAfter may be shown to callers, Err (the cause) never is.
// Any error that is not an *AppError is reported as CodeInternal.
type AppError struct {
	ResponseCode
	Data       any
	RetryAfter time.Duration
	Err        error
}

// NewAppError starts an error from a catalog entry.
func NewAppError(rc ResponseCode) *AppError { return &AppError{ResponseCode: rc} }

// WithRetryAfter tells the caller when to retry (rate limits).
func (e *AppError) WithRetryAfter(d time.Duration) *AppError {
	e.RetryAfter = d
	return e
}

// WithCause attaches the underlying error; it is logged, never returned.
func (e *AppError) WithCause(err error) *AppError {
	e.Err = err
	return e
}

func (e *AppError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *AppError) Unwrap() error { return e.Err }

// AsAppError returns the *AppError in err's chain, if any.
func AsAppError(err error) (*AppError, bool) {
	var e *AppError
	if errors.As(err, &e) {
		return e, true
	}
	return nil, false
}
