package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gabrielAnFran/pos-os-service/internal/presentation/dto"
	"github.com/gin-gonic/gin"
)

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic recovered", "error", recovered, "path", c.Request.URL.Path)
		c.AbortWithStatusJSON(http.StatusInternalServerError, dto.NewProblem(
			http.StatusInternalServerError,
			"Internal Server Error",
			"an unexpected error occurred",
			c.Request.URL.Path,
		))
	})
}
