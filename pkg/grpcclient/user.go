package grpcclient

import "context"

type UserClient interface {
	GetMSISDN(ctx context.Context, driverID string) (string, error)
}
