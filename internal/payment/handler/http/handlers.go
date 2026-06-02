package http

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/farid/payment-service/pkg/logger"
	"github.com/farid/payment-service/pkg/midtrans"
	"github.com/farid/payment-service/pkg/utils"
)

func (h *paymentHandler) createQrisIntent(c *gin.Context) {
	var body createIntentReq
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.Error(c, err)
		return
	}
	out, err := h.uc.CreateQrisIntent(c.Request.Context(), body.InvoiceID, body.AmountIDR)
	if err != nil {
		renderError(c, err)
		return
	}
	utils.Created(c, toIntentDTO(out), "QRIS intent created")
}

func (h *paymentHandler) getPayment(c *gin.Context) {
	out, err := h.uc.GetPayment(c.Request.Context(), c.Param("id"))
	if err != nil {
		renderError(c, err)
		return
	}
	utils.OK(c, toPaymentDTO(out), "payment retrieved successfully")
}

// midtransWebhook receives the Midtrans terminal-status callback. The handler
// reads the raw body, verifies the HMAC signature against MIDTRANS_WEBHOOK_SECRET,
// and dispatches to the usecase. We always return 200 to Midtrans on signature
// pass — even if the underlying status is "pending" — so they don't keep retrying.
// 401 is returned only on signature failure (Midtrans treats that as a hard fail).
func (h *paymentHandler) midtransWebhook(c *gin.Context) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "BAD_REQUEST", "message": "cannot read body"})
		return
	}

	n, err := midtrans.ParseAndVerify(raw, h.webhookSecret)
	if err != nil {
		if errors.Is(err, midtrans.ErrSignatureInvalid) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "SIGNATURE_INVALID"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "BAD_PAYLOAD", "message": err.Error()})
		return
	}

	if err := h.uc.HandleWebhook(c.Request.Context(), n); err != nil {
		// Log but still return 200 — Midtrans requires 200 to stop retrying.
		// NOT_FOUND (unknown order_id) is expected for test notifications.
		logger.Warn(c.Request.Context(), "webhook processing error (returning 200 to Midtrans)",
			map[string]interface{}{logger.ErrorKey: err.Error(), "order_id": n.OrderID})
	}
	utils.OK(c, gin.H{"status": "ok"}, "webhook accepted")
}
