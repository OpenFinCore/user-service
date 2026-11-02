package service

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"ofc-user-service/logger"
	"ofc-user-service/models"
)

// Response is the single envelope every endpoint returns, success or error:
//
//	{"code": 0, "message": "Success", "data": {...}}
//	{"code": 1000, "message": "Service Provider Id Header is required", "data": null}
type Response struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

var httpStatusByKind = map[models.ErrorKind]int{
	models.KindValidation:  http.StatusBadRequest,
	models.KindNotFound:    http.StatusNotFound,
	models.KindConflict:    http.StatusConflict,
	models.KindForbidden:   http.StatusForbidden,
	models.KindRateLimited: http.StatusTooManyRequests,
	models.KindUnavailable: http.StatusServiceUnavailable,
	models.KindInternal:    http.StatusInternalServerError,
}

// WriteSuccess writes data in the success envelope (code 0).
func WriteSuccess(w http.ResponseWriter, status int, data any) {
	writeEnvelope(w, status, Response{Code: models.CodeSuccess.Code, Message: models.CodeSuccess.Message, Data: data})
}

// WriteError writes err in the error envelope. Only *AppError details reach
// the client; anything else is logged and returned as CodeInternal.
func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	requestID := RequestIDFrom(r)

	appErr, ok := models.AsAppError(err)
	if !ok {
		logger.LogError(requestID, err)
		appErr = models.NewAppError(models.CodeInternal)
	}

	status, known := httpStatusByKind[appErr.Kind]
	if !known {
		status = http.StatusInternalServerError
	}
	if ok && status >= 500 {
		// Keep the cause in our logs; the client only sees the message.
		logger.LogError(requestID, err)
	}
	if appErr.RetryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(appErr.RetryAfter.Seconds())))
	}
	writeEnvelope(w, status, Response{Code: appErr.Code, Message: appErr.Message, Data: appErr.Data})
}

func writeEnvelope(w http.ResponseWriter, status int, body Response) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// DecodeJSON strictly decodes a small JSON request body into dst.
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return models.NewAppError(models.CodeRequestBodyTooLarge)
		}
		return models.NewAppError(models.CodeInvalidRequestBody)
	}
	if dec.More() {
		return models.NewAppError(models.CodeInvalidRequestBody)
	}
	return nil
}
