package http

import (
	"net/http"

	"github.com/gin-gonic/gin"

	apperror "github.com/farid/payment-service/pkg/error"
)

func renderError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if ae, ok := err.(*apperror.AppError); ok {
		switch ae.Code {
		case "VALIDATION":
			c.JSON(http.StatusBadRequest, gin.H{"error": ae.Code, "message": ae.Message})
			return
		case "NOT_FOUND":
			c.JSON(http.StatusNotFound, gin.H{"error": ae.Code, "message": ae.Message})
			return
		case "CONFLICT":
			c.JSON(http.StatusConflict, gin.H{"error": ae.Code, "message": ae.Message})
			return
		case "UNAUTHENTICATED":
			c.JSON(http.StatusUnauthorized, gin.H{"error": ae.Code, "message": ae.Message})
			return
		case "UPSTREAM_DOWN":
			c.JSON(http.StatusBadGateway, gin.H{"error": ae.Code, "message": ae.Message})
			return
		}
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL", "message": err.Error()})
}
