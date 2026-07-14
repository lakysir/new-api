// Order state machine and persistence. States and transitions mirror
// architecture §11 (aligned with PRD §5.4). All multi-field transitions go
// through ApplyTransition, which uses an optimistic lock_version to serialize
// concurrent writers; controllers never mutate state columns directly.
package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// Order states (architecture §11).
const (
	OrderCreated       = "CREATED"
	OrderFundsReserved = "FUNDS_RESERVED"
	OrderMatching      = "MATCHING"
	OrderOffered       = "OFFERED"
	OrderReserved      = "RESERVED"
	OrderDataReady     = "DATA_READY"
	OrderRunning       = "RUNNING"
	OrderResultReady   = "RESULT_READY"
	OrderVerifying     = "VERIFYING"
	OrderSettled       = "SETTLED"
	OrderCancelled     = "CANCELLED"
	OrderFailed        = "FAILED"
	OrderTimedOut      = "TIMED_OUT"
	OrderRefunded      = "REFUNDED"
	OrderDisputed      = "DISPUTED"
	OrderResolved      = "RESOLVED"
)

var (
	// ErrOrderNotFound is returned when an order id is missing.
	ErrOrderNotFound = errors.New("order not found")
	// ErrIllegalTransition is returned when a state transition is not allowed.
	ErrIllegalTransition = errors.New("illegal order state transition")
	// ErrOrderConcurrentUpdate is returned when the optimistic lock check fails.
	ErrOrderConcurrentUpdate = errors.New("order was updated concurrently")
)

// allowedTransitions is the adjacency list of legal order transitions. Any move
// not listed here is rejected by ApplyTransition (property: illegal moves fail).
var allowedTransitions = map[string]map[string]bool{
	OrderCreated:       {OrderFundsReserved: true, OrderCancelled: true},
	OrderFundsReserved: {OrderMatching: true, OrderCancelled: true},
	OrderMatching:      {OrderOffered: true, OrderCancelled: true, OrderTimedOut: true},
	OrderOffered:       {OrderReserved: true, OrderMatching: true, OrderCancelled: true, OrderTimedOut: true},
	OrderReserved:      {OrderDataReady: true, OrderMatching: true, OrderFailed: true, OrderTimedOut: true},
	OrderDataReady:     {OrderRunning: true, OrderFailed: true, OrderTimedOut: true},
	OrderRunning:       {OrderResultReady: true, OrderFailed: true, OrderTimedOut: true, OrderDisputed: true},
	OrderResultReady:   {OrderVerifying: true, OrderFailed: true, OrderDisputed: true},
	OrderVerifying:     {OrderSettled: true, OrderDisputed: true, OrderFailed: true},
	OrderFailed:        {OrderMatching: true, OrderRefunded: true, OrderDisputed: true},
	OrderTimedOut:      {OrderRefunded: true},
	OrderCancelled:     {OrderRefunded: true},
	OrderDisputed:      {OrderResolved: true},
	OrderResolved:      {OrderSettled: true, OrderRefunded: true},
	// Terminal states have no outgoing transitions.
	OrderSettled:  {},
	OrderRefunded: {},
}

// CanTransition reports whether from -> to is a legal order transition.
func CanTransition(from, to string) bool {
	next, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

// Order is a client's paid execution request against a fixed script version.
type Order struct {
	Id             string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	ClientId       int    `json:"client_id" gorm:"index;not null"`
	ScriptId       int    `json:"script_id" gorm:"index;not null"`
	Version        int    `json:"version" gorm:"not null"`
	State          string `json:"state" gorm:"type:varchar(24);index;default:CREATED"`
	InputHash      string `json:"input_hash" gorm:"type:varchar(80)"`
	IdempotencyKey string `json:"idempotency_key" gorm:"type:varchar(80);uniqueIndex"`
	// ChosenNodeId is the provider node the client selected (empty = scheduler
	// picks the cheapest eligible node within budget).
	ChosenNodeId      string `json:"chosen_node_id" gorm:"type:varchar(64)"`
	MaxAmountMicros   int64  `json:"max_amount_micros" gorm:"default:0"`
	FinalAmountMicros int64  `json:"final_amount_micros" gorm:"default:0"`
	LockVersion       int64  `json:"lock_version" gorm:"default:0"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime"`
	// LastError is not persisted; controllers populate it on read from the
	// latest task attempt's error_code so failed orders can show a real reason.
	LastError string `json:"last_error,omitempty" gorm:"-"`
}

func (Order) TableName() string { return "orders" }

// OrderPriceSnapshot freezes the price breakdown at order creation so later
// template changes never affect an existing order (PRD §5.5.5). One row per
// order.
type OrderPriceSnapshot struct {
	OrderId                  string `json:"order_id" gorm:"primaryKey;type:varchar(64)"`
	Currency                 string `json:"currency" gorm:"type:varchar(8)"`
	ProviderAmountMicros     int64  `json:"provider_amount_micros"`
	AuthorAmountMicros       int64  `json:"author_amount_micros"`
	PlatformFeeMicros        int64  `json:"platform_fee_micros"`
	RelayFeeReservedMicros   int64  `json:"relay_fee_reserved_micros"`
	StorageFeeReservedMicros int64  `json:"storage_fee_reserved_micros"`
	RiskReserveMicros        int64  `json:"risk_reserve_micros"`
	MaxCustomerAmountMicros  int64  `json:"max_customer_amount_micros"`
	PricingRuleVersion       string `json:"pricing_rule_version" gorm:"type:varchar(32)"`
	CreatedAt                int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (OrderPriceSnapshot) TableName() string { return "order_price_snapshots" }

// FindOrderByIdempotencyKey returns an existing order for a client + key, or
// (nil, nil) if none. Used to make order creation idempotent (T-002).
func FindOrderByIdempotencyKey(clientId int, key string) (*Order, error) {
	if key == "" {
		return nil, nil
	}
	var o Order
	err := DB.Where("client_id = ? AND idempotency_key = ?", clientId, key).First(&o).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// CreateOrderWithSnapshot creates an order and its price snapshot atomically. If
// the idempotency key already exists, the existing order is returned unchanged
// (no duplicate funds reservation).
func CreateOrderWithSnapshot(o *Order, snap *OrderPriceSnapshot) (*Order, bool, error) {
	if existing, err := FindOrderByIdempotencyKey(o.ClientId, o.IdempotencyKey); err != nil {
		return nil, false, err
	} else if existing != nil {
		return existing, false, nil
	}
	created := false
	err := DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(o).Error; err != nil {
			return err
		}
		snap.OrderId = o.Id
		if err := tx.Create(snap).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		// Lost a race on the unique idempotency key: return the winner.
		if isUniqueConstraintErr(err) {
			existing, ferr := FindOrderByIdempotencyKey(o.ClientId, o.IdempotencyKey)
			if ferr == nil && existing != nil {
				return existing, false, nil
			}
		}
		return nil, false, err
	}
	return o, created, nil
}

// GetOrder returns an order by id.
func GetOrder(id string) (*Order, error) {
	var o Order
	if err := DB.Where("id = ?", id).First(&o).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrOrderNotFound
		}
		return nil, err
	}
	return &o, nil
}

// ApplyTransition moves an order to newState if the transition is legal, using
// an optimistic lock on lock_version. Extra column updates (e.g. final amount)
// can be passed via extra. Returns ErrIllegalTransition or
// ErrOrderConcurrentUpdate on failure. This is the only path that mutates
// order.state.
func ApplyTransition(orderId, newState string, extra map[string]any) (*Order, error) {
	var updated *Order
	err := DB.Transaction(func(tx *gorm.DB) error {
		var o Order
		if err := tx.Where("id = ?", orderId).First(&o).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrOrderNotFound
			}
			return err
		}
		if !CanTransition(o.State, newState) {
			return ErrIllegalTransition
		}
		updates := map[string]any{
			"state":        newState,
			"lock_version": o.LockVersion + 1,
			"updated_at":   time.Now().Unix(),
		}
		for k, v := range extra {
			updates[k] = v
		}
		res := tx.Model(&Order{}).
			Where("id = ? AND lock_version = ?", orderId, o.LockVersion).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrOrderConcurrentUpdate
		}
		o.State = newState
		o.LockVersion++
		updated = &o
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// GetOrderPriceSnapshot returns the frozen price snapshot for an order.
func GetOrderPriceSnapshot(orderId string) (*OrderPriceSnapshot, error) {
	var s OrderPriceSnapshot
	if err := DB.Where("order_id = ?", orderId).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

// NewOrderId returns a new order id.
func NewOrderId() string { return "ord_" + common.GetUUID() }
