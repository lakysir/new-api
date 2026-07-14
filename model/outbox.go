// Transactional Outbox. Business state changes and the control events they emit
// are written in the same DB transaction (architecture §22.4), so an event is
// never lost nor published without its state change. A publisher drains
// unpublished rows; consumers dedupe by event_id (architecture §8.3).
package model

import (
	"errors"
	"time"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// OutboxEvent is one control event awaiting (or past) publication.
type OutboxEvent struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	EventId     string `json:"event_id" gorm:"type:varchar(64);uniqueIndex;not null"`
	Type        string `json:"type" gorm:"type:varchar(48);index;not null"`
	AggregateId string `json:"aggregate_id" gorm:"type:varchar(64);index"` // e.g. order/task id
	Payload     string `json:"payload" gorm:"type:text"`                   // JSON, metadata only
	PublishedAt int64  `json:"published_at" gorm:"index;default:0"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (OutboxEvent) TableName() string { return "outbox_events" }

// ProcessedEvent records event_ids a consumer has already handled, giving
// exactly-once effect on top of at-least-once delivery.
type ProcessedEvent struct {
	EventId     string `json:"event_id" gorm:"primaryKey;type:varchar(64)"`
	Consumer    string `json:"consumer" gorm:"primaryKey;type:varchar(48)"`
	ProcessedAt int64  `json:"processed_at" gorm:"autoCreateTime"`
}

func (ProcessedEvent) TableName() string { return "processed_events" }

// NewEventId returns a fresh event id.
func NewEventId() string { return "evt_" + common.GetUUID() }

// EnqueueOutboxTx appends an event within an existing transaction. Callers that
// change state and emit an event must use this inside their own tx so the two
// commit together.
func EnqueueOutboxTx(tx *gorm.DB, eventType, aggregateId, payload string) (*OutboxEvent, error) {
	e := &OutboxEvent{
		EventId:     NewEventId(),
		Type:        eventType,
		AggregateId: aggregateId,
		Payload:     payload,
	}
	if err := tx.Create(e).Error; err != nil {
		return nil, err
	}
	return e, nil
}

// FetchUnpublished returns up to limit unpublished events oldest-first.
func FetchUnpublished(limit int) ([]OutboxEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	var events []OutboxEvent
	err := DB.Where("published_at = 0").Order("id asc").Limit(limit).Find(&events).Error
	return events, err
}

// MarkPublished stamps an event as published after the publisher hands it off.
func MarkPublished(eventId string) error {
	return DB.Model(&OutboxEvent{}).Where("event_id = ?", eventId).
		Update("published_at", time.Now().Unix()).Error
}

// GetOutboxEvent returns one event by its stable event id.
func GetOutboxEvent(eventId string) (*OutboxEvent, error) {
	var event OutboxEvent
	if err := DB.Where("event_id = ?", eventId).First(&event).Error; err != nil {
		return nil, err
	}
	return &event, nil
}

// ErrAlreadyProcessed is returned by MarkProcessed when the (event, consumer)
// pair was already recorded — the caller should skip its side effects.
var ErrAlreadyProcessed = errors.New("event already processed by consumer")

// MarkProcessedTx claims an event for a consumer within a transaction. It
// returns ErrAlreadyProcessed if this consumer already handled the event, so
// the caller can make its handler idempotent by checking this first.
func MarkProcessedTx(tx *gorm.DB, eventId, consumer string) error {
	err := tx.Create(&ProcessedEvent{EventId: eventId, Consumer: consumer}).Error
	if err != nil {
		if isUniqueConstraintErr(err) {
			return ErrAlreadyProcessed
		}
		return err
	}
	return nil
}

// ConsumeOnce runs handler exactly once for (eventId, consumer). The claim and
// the handler side effects commit in one transaction; a duplicate delivery is a
// no-op success. handler must use the provided tx for its writes.
func ConsumeOnce(eventId, consumer string, handler func(tx *gorm.DB) error) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		if err := MarkProcessedTx(tx, eventId, consumer); err != nil {
			if errors.Is(err, ErrAlreadyProcessed) {
				return nil // already done; skip side effects
			}
			return err
		}
		return handler(tx)
	})
}
