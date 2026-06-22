package subscription

import "context"

type Repository interface {
	ListSubscriptions(context.Context) ([]Subscription, error)
	CreateSubscription(context.Context, Subscription, string) (Subscription, error)
	UpdateSubscription(context.Context, int64, Input) (Subscription, error)
	RotateSubscriptionToken(context.Context, int64, string, string) (Subscription, error)
	DeleteSubscription(context.Context, int64) error
	FindSubscriptionByTokenHash(context.Context, string) (Subscription, error)
}
