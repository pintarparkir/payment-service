// Package grpc adapts the PaymentService proto contract to the usecase layer.
package grpc

import (
	"google.golang.org/grpc"

	paymentv1 "github.com/farid/payment-service/api/proto/payment/v1"
	"github.com/farid/payment-service/internal/payment/usecase"
)

type Server struct {
	paymentv1.UnimplementedPaymentServiceServer
	uc usecase.PaymentUsecase
}

func Register(s grpc.ServiceRegistrar, uc usecase.PaymentUsecase) {
	paymentv1.RegisterPaymentServiceServer(s, &Server{uc: uc})
}
