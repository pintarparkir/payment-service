package http

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/farid/payment-service/pkg/midtrans"
)

func (h *paymentHandler) createQrisIntent(c *gin.Context) {
	var body createIntentReq
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "BAD_REQUEST", "message": err.Error()})
		return
	}
	out, err := h.uc.CreateQrisIntent(c.Request.Context(), body.InvoiceID, body.AmountIDR)
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusCreated, toIntentDTO(out))
}

func (h *paymentHandler) getPayment(c *gin.Context) {
	out, err := h.uc.GetPayment(c.Request.Context(), c.Param("id"))
	if err != nil {
		renderError(c, err)
		return
	}
	c.JSON(http.StatusOK, toPaymentDTO(out))
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
		// 5xx — Midtrans will retry. Logging happens in the usecase already.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
