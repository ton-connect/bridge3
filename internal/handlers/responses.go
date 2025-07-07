package handlers

import "net/http"

// Response represents a standard HTTP response
type Response struct {
	Message    string `json:"message,omitempty"`
	StatusCode int    `json:"statusCode,omitempty"`
}

// SuccessResponse returns a success response
func SuccessResponse() Response {
	return Response{
		Message:    "OK",
		StatusCode: http.StatusOK,
	}
}

// ErrorResponse returns an error response
func ErrorResponse(message string, statusCode int) Response {
	return Response{
		Message:    message,
		StatusCode: statusCode,
	}
}
