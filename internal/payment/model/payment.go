package model

import "time"

type PaymentStatus string

const (
	PaymentPending  PaymentStatus = "PENDING"
	PaymentPaid     PaymentStatus = "PAID"
	PaymentFailed   PaymentStatus = "FAILED"
	PaymentRefunded PaymentStatus = "REFUNDED"
)

type Payment struct {
	ID          string
	InvoiceID   string
	Method      string // QRIS
	Status      PaymentStatus
	PgReference string
	AmountIDR   int64
	CreatedAt   time.Time
	PaidAt      *time.Time
}
