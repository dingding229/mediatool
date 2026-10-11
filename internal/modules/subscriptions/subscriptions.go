package subscriptions

import "context"

type Subscription struct {
	ID, Title, MediaType string
	Enabled              bool
	Season               int
	Episodes             []int
}
type Decision struct {
	Matched bool
	Reasons []string
}
type Service interface {
	Evaluate(context.Context, string) error
}
