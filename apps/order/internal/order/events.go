package order

import (
	"context"
	"encoding/json"
	"time"

	"github.com/portfolio/pf-commerce/packages/money"
)

// Event types for the order aggregate only. Catalog stays CRUD.
const (
	EventOrderCreated      EventType = "OrderCreated"
	EventInventoryReserved EventType = "InventoryReserved"
	EventInventoryShortage EventType = "InventoryShortage"
	EventPaymentRecorded   EventType = "PaymentRecorded"
	EventPaymentFailed     EventType = "PaymentFailed"
	EventOrderCancelled    EventType = "OrderCancelled"
	EventOrderShipped      EventType = "OrderShipped"
)

type EventType string

type NewEvent struct {
	Type EventType
	Time time.Time
	Data any
}

type RecordedEvent struct {
	ID       string
	StreamID string
	Version  int
	Type     EventType
	Time     time.Time
	Data     json.RawMessage
}

type EventStore interface {
	Append(ctx context.Context, streamID string, expectedVersion int, events []NewEvent) error
	Load(ctx context.Context, streamID string) ([]RecordedEvent, error)
	LoadAll(ctx context.Context) ([]RecordedEvent, error)
	RebuildProjections(ctx context.Context) error
}

type OutboxMessage struct {
	ID          string
	AggregateID string
	Type        string
	Payload     json.RawMessage
	CreatedAt   time.Time
	PublishedAt *time.Time
}

type OutboxStore interface {
	ListUnpublished(ctx context.Context, limit int) ([]OutboxMessage, error)
	MarkPublished(ctx context.Context, ids []string, at time.Time) error
}

type Persistence interface {
	Repository
	EventStore
	OutboxStore
}

type OrderCreatedData struct {
	BuyerSub       string `json:"buyerSub"`
	IdempotencyKey string `json:"idempotencyKey"`
	AmountMinor    int64  `json:"amountMinor"`
	Currency       string `json:"currency"`
	Lines          []Line `json:"lines"`
}

type PaymentRecordedData struct {
	PaymentID string `json:"paymentId"`
}

type CancelledData struct {
	Reason string `json:"reason"`
}

func Fold(events []RecordedEvent) (Order, error) {
	var o Order
	for _, e := range events {
		next, err := Apply(o, e)
		if err != nil {
			return Order{}, err
		}
		o = next
	}
	return o, nil
}

func Apply(o Order, e RecordedEvent) (Order, error) {
	switch e.Type {
	case EventOrderCreated:
		var d OrderCreatedData
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return Order{}, err
		}
		amt, err := money.New(d.AmountMinor, d.Currency)
		if err != nil {
			return Order{}, err
		}
		o = Order{
			ID:             e.StreamID,
			BuyerSub:       d.BuyerSub,
			Status:         StatusPending,
			Amount:         amt,
			IdempotencyKey: d.IdempotencyKey,
			Lines:          append([]Line{}, d.Lines...),
			CreatedAt:      e.Time,
			UpdatedAt:      e.Time,
		}
	case EventInventoryReserved, EventInventoryShortage:
		o.UpdatedAt = e.Time
	case EventPaymentRecorded:
		var d PaymentRecordedData
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return Order{}, err
		}
		o.Status = StatusPaid
		o.PaymentID = d.PaymentID
		o.UpdatedAt = e.Time
	case EventPaymentFailed:
		o.UpdatedAt = e.Time
	case EventOrderCancelled:
		var d CancelledData
		if len(e.Data) > 0 {
			_ = json.Unmarshal(e.Data, &d)
		}
		o.Status = StatusCancelled
		o.CancelReason = d.Reason
		o.UpdatedAt = e.Time
	case EventOrderShipped:
		o.Status = StatusShipped
		o.UpdatedAt = e.Time
	default:
		return Order{}, ErrInvalid
	}
	return o, nil
}

func DecideCreate(existing Order, in CheckoutInput, lines []Line, totalMinor int64, currency string, now time.Time) ([]NewEvent, error) {
	if existing.ID != "" {
		return nil, ErrConflict
	}
	if in.BuyerSub == "" || in.IdempotencyKey == "" || len(lines) == 0 {
		return nil, ErrInvalid
	}
	return []NewEvent{{
		Type: EventOrderCreated,
		Time: now,
		Data: OrderCreatedData{
			BuyerSub: in.BuyerSub, IdempotencyKey: in.IdempotencyKey,
			AmountMinor: totalMinor, Currency: currency, Lines: lines,
		},
	}}, nil
}

func DecideReserveOK(o Order, now time.Time) ([]NewEvent, error) {
	if o.Status != StatusPending {
		return nil, ErrInvalidTransition
	}
	return []NewEvent{{Type: EventInventoryReserved, Time: now, Data: map[string]any{}}}, nil
}

func DecideShortage(o Order, now time.Time) ([]NewEvent, error) {
	if o.Status != StatusPending {
		return nil, ErrInvalidTransition
	}
	return []NewEvent{
		{Type: EventInventoryShortage, Time: now, Data: map[string]any{"reason": ReasonShortage}},
		{Type: EventOrderCancelled, Time: now, Data: CancelledData{Reason: ReasonShortage}},
	}, nil
}

func DecidePaymentOK(o Order, paymentID string, now time.Time) ([]NewEvent, error) {
	if o.Status != StatusPending {
		return nil, ErrInvalidTransition
	}
	if paymentID == "" {
		return nil, ErrInvalid
	}
	return []NewEvent{{
		Type: EventPaymentRecorded, Time: now, Data: PaymentRecordedData{PaymentID: paymentID},
	}}, nil
}

func DecidePaymentFail(o Order, now time.Time) ([]NewEvent, error) {
	if o.Status != StatusPending {
		return nil, ErrInvalidTransition
	}
	return []NewEvent{
		{Type: EventPaymentFailed, Time: now, Data: map[string]any{"reason": ReasonPayment}},
		{Type: EventOrderCancelled, Time: now, Data: CancelledData{Reason: ReasonPayment}},
	}, nil
}

func DecideCancel(o Order, reason string, now time.Time) ([]NewEvent, error) {
	if o.Status == StatusShipped {
		return nil, ErrInvalidTransition
	}
	if o.Status == StatusCancelled {
		return nil, nil
	}
	if reason == "" {
		reason = "cancelled"
	}
	return []NewEvent{{Type: EventOrderCancelled, Time: now, Data: CancelledData{Reason: reason}}}, nil
}

func DecideShip(o Order, now time.Time) ([]NewEvent, error) {
	if o.Status != StatusPaid {
		return nil, ErrInvalidTransition
	}
	return []NewEvent{{Type: EventOrderShipped, Time: now, Data: map[string]any{}}}, nil
}

// NotifyTopic is the mail type for outbox rows. PaymentFailed is covered by OrderCancelled.
func NotifyTopic(t EventType) (string, bool) {
	switch t {
	case EventPaymentRecorded:
		return "OrderPaid", true
	case EventOrderCancelled:
		return "OrderCancelled", true
	default:
		return "", false
	}
}
