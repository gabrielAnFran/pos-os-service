package middleware

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"github.com/gabrielAnFran/pos-os-service/internal/domain/repositories"
	"github.com/gabrielAnFran/pos-os-service/internal/presentation/dto"
	"github.com/gin-gonic/gin"
)

const IdempotencyKeyHeader = "Idempotency-Key"

type bodyCaptureWriter struct {
	gin.ResponseWriter
	buf *bytes.Buffer
}

func (w *bodyCaptureWriter) Write(b []byte) (int, error) {
	w.buf.Write(b)
	return w.ResponseWriter.Write(b)
}

// Idempotency replays a previously stored response when the same
// Idempotency-Key + request body hash is seen again, and returns 409 when the
// same key is reused with a different body. Requests without the header are
// passed through untouched.
func Idempotency(repo repositories.IdempotencyRepository) gin.HandlerFunc {
	return func(c *gin.Context) {
		key := c.GetHeader(IdempotencyKeyHeader)
		if key == "" {
			c.Next()
			return
		}

		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusBadRequest, dto.NewProblem(http.StatusBadRequest, "Bad Request", "unable to read request body", c.Request.URL.Path))
			return
		}
		c.Request.Body = io.NopCloser(bytes.NewReader(bodyBytes))

		hash := sha256.Sum256(bodyBytes)
		hashHex := hex.EncodeToString(hash[:])

		existing, err := repo.Get(c.Request.Context(), key)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, dto.NewProblem(http.StatusInternalServerError, "Internal Server Error", "idempotency lookup failed", c.Request.URL.Path))
			return
		}
		if existing != nil {
			if existing.RequestHash != hashHex {
				c.AbortWithStatusJSON(http.StatusConflict, dto.NewProblem(http.StatusConflict, "Conflict", "Idempotency-Key was already used with a different request body", c.Request.URL.Path))
				return
			}
			c.Data(existing.StatusCode, "application/json", existing.ResponseBody)
			c.Abort()
			return
		}

		writer := &bodyCaptureWriter{ResponseWriter: c.Writer, buf: &bytes.Buffer{}}
		c.Writer = writer

		c.Next()

		if c.Writer.Status() >= 200 && c.Writer.Status() < 300 {
			rec := &repositories.IdempotencyRecord{
				Key:          key,
				RequestHash:  hashHex,
				ResponseBody: writer.buf.Bytes(),
				StatusCode:   c.Writer.Status(),
			}
			_ = repo.Save(c.Request.Context(), rec)
		}
	}
}
