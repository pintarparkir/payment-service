package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	pkgjwt "github.com/farid/payment-service/pkg/jwt"
)

// jwtMiddleware parses + verifies the Bearer JWT, sets driver_id in context.
// Same pattern as reservation-service.
func (h *paymentHandler) jwtMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := c.GetHeader("Authorization")
		if !strings.HasPrefix(raw, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": "UNAUTHENTICATED", "message": "missing bearer token"})
			return
		}
		claims, err := pkgjwt.Parse(strings.TrimPrefix(raw, "Bearer "), h.jwtKey)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized,
				gin.H{"error": "UNAUTHENTICATED", "message": err.Error()})
			return
		}
		c.Set(ctxDriverID, claims.Sub)
		c.Next()
	}
}
