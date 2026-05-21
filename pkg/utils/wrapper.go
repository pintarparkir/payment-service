package utils

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	apperror "github.com/farid/payment-service/pkg/error"
	"github.com/farid/payment-service/pkg/logger"
)

type ResponseWrapper struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   *ErrorInfo  `json:"error,omitempty"`
	Message string      `json:"message"`
	Code    int         `json:"code"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type AuditMeta struct {
	Method      string    `json:"method"`
	Path        string    `json:"path"`
	StatusCode  int       `json:"status_code"`
	ClientIP    string    `json:"client_ip"`
	Timestamp   time.Time `json:"timestamp"`
	RequestSize int       `json:"request_size"`
}

func OK(c *gin.Context, data interface{}, message string) {
	Response(c, data, message, http.StatusOK)
}

func Created(c *gin.Context, data interface{}, message string) {
	Response(c, data, message, http.StatusCreated)
}

func Response(c *gin.Context, data interface{}, message string, statusCode int) {
	success := statusCode < http.StatusBadRequest

	logAudit(c, statusCode, nil)

	result := ResponseWrapper{
		Success: success,
		Data:    data,
		Message: message,
		Code:    statusCode,
	}

	c.JSON(statusCode, result)
}

func Error(c *gin.Context, err error) {
	if err == nil {
		Response(c, nil, "unknown error", http.StatusInternalServerError)
		return
	}

	var statusCode int
	var errorCode string
	var errorMessage string

	var appErr *apperror.AppError
	if errors.As(err, &appErr) {
		errorCode = appErr.Code
		errorMessage = appErr.Message

		switch appErr.Code {
		case "VALIDATION":
			statusCode = http.StatusBadRequest
		case "UNAUTHENTICATED":
			statusCode = http.StatusUnauthorized
		case "NOT_FOUND":
			statusCode = http.StatusNotFound
		case "CONFLICT", "DOUBLE_BOOK", "LOCK_UNAVAILABLE":
			statusCode = http.StatusConflict
		case "INVALID_STATE", "IDEMPOTENCY_REPLAY":
			statusCode = http.StatusConflict
		case "UPSTREAM_DOWN":
			statusCode = http.StatusServiceUnavailable
		default:
			statusCode = http.StatusInternalServerError
		}
	} else {
		statusCode = http.StatusInternalServerError
		errorCode = "INTERNAL"
		errorMessage = err.Error()
	}

	logAudit(c, statusCode, &ErrorInfo{Code: errorCode, Message: errorMessage})

	result := ResponseWrapper{
		Success: false,
		Error: &ErrorInfo{
			Code:    errorCode,
			Message: errorMessage,
		},
		Message: errorMessage,
		Code:    statusCode,
	}

	c.JSON(statusCode, result)
}

func logAudit(c *gin.Context, statusCode int, errInfo *ErrorInfo) {
	meta := AuditMeta{
		Method:      c.Request.Method,
		Path:        c.Request.URL.Path,
		StatusCode:  statusCode,
		ClientIP:    c.ClientIP(),
		Timestamp:   time.Now(),
		RequestSize: int(c.Request.ContentLength),
	}

	metaJSON, _ := json.Marshal(meta)
	fields := map[string]interface{}{
		"meta": string(metaJSON),
	}

	if errInfo != nil {
		fields["error_code"] = errInfo.Code
		fields["error_message"] = errInfo.Message
		logger.Warn(c.Request.Context(), "http_response_error", fields)
	} else {
		logger.Info(c.Request.Context(), "http_response_ok", fields)
	}
}
