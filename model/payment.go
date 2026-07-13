package model

import (
	"errors"

	"gorm.io/gorm"
)

// Payment / withdrawal statuses.
const (
	PaymentStatusPending   = "pending"
	PaymentStatusConfirmed = "confirmed"
	PaymentStatusFailed    = "failed"
)

// PaymentTransaction records an on-chain deposit or withdrawal, normalized
// across networks. ExternalId is unique per channel so duplicate callbacks and
// re-scans are idempotent (architecture §14.5).
type PaymentTransaction struct {
	Id            int    `json:"id" gorm:"primaryKey;autoIncrement"`
	Channel       string `json:"channel" gorm:"type:varchar(32);uniqueIndex:idx_channel_ext;not null"`
	ExternalId    string `json:"external_id" gorm:"type:varchar(128);uniqueIndex:idx_channel_ext;not null"`
	Direction     string `json:"direction" gorm:"type:varchar(8);index"` // deposit | withdrawal
	AccountId     string `json:"account_id" gorm:"type:varchar(64);index"`
	AmountMicros  int64  `json:"amount_micros"`
	Confirmations int    `json:"confirmations" gorm:"default:0"`
	Status        string `json:"status" gorm:"type:varchar(16);index;default:pending"`
	CreatedAt     int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt     int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (PaymentTransaction) TableName() string { return "payment_transactions" }

// ErrPaymentExists is returned when a channel+external_id already exists.
var ErrPaymentExists = errors.New("payment transaction already exists")

// RecordPaymentIfNew inserts a payment transaction, returning (row, created).
// If the (channel, external_id) already exists, the existing row is returned
// with created=false — this is the idempotency guard against duplicate
// callbacks and re-scanned deposits.
func RecordPaymentIfNew(p *PaymentTransaction) (*PaymentTransaction, bool, error) {
	var existing PaymentTransaction
	err := DB.Where("channel = ? AND external_id = ?", p.Channel, p.ExternalId).First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}
	if err := DB.Create(p).Error; err != nil {
		if isUniqueConstraintErr(err) {
			if rerr := DB.Where("channel = ? AND external_id = ?", p.Channel, p.ExternalId).First(&existing).Error; rerr == nil {
				return &existing, false, nil
			}
		}
		return nil, false, err
	}
	return p, true, nil
}

// UpdatePaymentConfirmations updates confirmations/status for a transaction.
func UpdatePaymentConfirmations(channel, externalId string, confirmations int, status string) error {
	res := DB.Model(&PaymentTransaction{}).
		Where("channel = ? AND external_id = ?", channel, externalId).
		Updates(map[string]any{"confirmations": confirmations, "status": status})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errors.New("payment transaction not found")
	}
	return nil
}

// GetPaymentTransaction returns a transaction by channel + external id.
func GetPaymentTransaction(channel, externalId string) (*PaymentTransaction, error) {
	var p PaymentTransaction
	if err := DB.Where("channel = ? AND external_id = ?", channel, externalId).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}
