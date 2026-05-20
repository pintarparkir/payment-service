package http

import (
	"github.com/gin-gonic/gin"

	"github.com/farid/payment-service/pkg/utils"
)

func renderError(c *gin.Context, err error) {
	utils.Error(c, err)
}
