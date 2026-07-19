package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const CorrelationIDHeader = "X-Correlation-ID"
const CorrelationIDKey = "correlation_id"

func Correlation() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader(CorrelationIDHeader)
		if id == "" {
			id = uuid.New().String()
		}
		c.Set(CorrelationIDKey, id)
		c.Writer.Header().Set(CorrelationIDHeader, id)
		c.Next()
	}
}

func CorrelationIDFromContext(c *gin.Context) string {
	if v, ok := c.Get(CorrelationIDKey); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}
