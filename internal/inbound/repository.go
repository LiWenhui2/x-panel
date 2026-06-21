package inbound

import "context"

type Repository interface {
	List(context.Context) ([]Inbound, error)
	Create(context.Context, Inbound) (Inbound, error)
}
