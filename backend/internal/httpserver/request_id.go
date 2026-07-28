package httpserver

import (
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/gin-gonic/gin"

	"k8s-aiops.local/backend/internal/requestctx"
)

const (
	requestIDHeader = "X-Request-ID"
	maxRequestIDLen = 128
)

func withRequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if requestID == "" || len(requestID) > maxRequestIDLen || !isVisibleASCII(requestID) {
			requestID = newRequestID()
		}

		metadata := requestctx.Metadata{RequestID: requestID}
		c.Request = c.Request.WithContext(requestctx.WithMetadata(c.Request.Context(), metadata))
		c.Header(requestIDHeader, requestID)
		c.Next()
	}
}

func newRequestID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic("secure random source is unavailable")
	}
	return hex.EncodeToString(buffer)
}

func isVisibleASCII(value string) bool {
	for _, char := range value {
		if char < 33 || char > 126 {
			return false
		}
	}
	return true
}
