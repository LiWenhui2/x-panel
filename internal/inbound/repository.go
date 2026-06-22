package inbound

import "context"

type Repository interface {
	List(context.Context) ([]Inbound, error)
	Create(context.Context, Inbound) (Inbound, error)
	Update(context.Context, int64, Inbound) (Inbound, error)
	AddUsedBytes(context.Context, int64, int64) error
}
