package notifications

import (
	"context"
	"encoding/json"
)

type Event struct {
	ID, Type, AggregateID string
	Payload               json.RawMessage
}
type NotificationChannel interface {
	Send(context.Context, Event) error
}
