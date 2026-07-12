package subscription

import "context"

type Repository interface {
	ListSubscriptions(context.Context) ([]Subscription, error)
	CreateSubscription(context.Context, Subscription, string) (Subscription, error)
	UpdateSubscription(context.Context, int64, Input) (Subscription, error)
	RotateSubscriptionToken(context.Context, int64, string, string, string) (Subscription, error)
	DeleteSubscription(context.Context, int64) error
	FindSubscriptionByToken(context.Context, string, string) (Subscription, error)
	SubscriptionToken(context.Context, int64) (Subscription, string, error)
}
