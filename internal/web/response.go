package web

import "github.com/gin-gonic/gin"

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(200, gin.H{"ok": true, "data": data, "error": nil})
}

func Fail(c *gin.Context, status int, code, message string, details ...any) {
	var d any
	if len(details) > 0 {
		d = details[0]
	}
	c.JSON(status, gin.H{"ok": false, "data": nil, "error": APIError{Code: code, Message: message, Details: d}})
}
