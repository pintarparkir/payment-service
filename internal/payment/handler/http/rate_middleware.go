package http

import (
	"fmt"
	"net/http"
	"time"

	"github.com/farid/payment-service/pkg/rate"
	"github.com/gin-gonic/gin"
)

func rateLimitDriver(lim rate.Limiter, endpoint string, limit int, window time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		driverID := c.GetString(ctxDriverID)
		if driverID == "" || lim == nil {
			c.Next()
			return
		}
		ok, retry, err := lim.Allow(c.Request.Context(), "rl:"+endpoint+":"+driverID, limit, window)
		if err != nil {
			c.Next()
			return
		}
		if !ok {
			c.Header("Retry-After", fmt.Sprintf("%d", int(retry.Seconds())))
			c.AbortWithStatus(http.StatusTooManyRequests)
			return
		}
		c.Next()
	}
}
