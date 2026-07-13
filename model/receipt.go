package model

import (
	"errors"

	"gorm.io/gorm"
)

// Receipt is a stored signed proof from one party for a task attempt. Only
// hashes and the signature are persisted — never plaintext (architecture §12:
// no prompt/result columns). (task_id, attempt, party) is unique.
type Receipt struct {
	Id           int    `json:"id" gorm:"primaryKey;autoIncrement"`
	TaskId       string `json:"task_id" gorm:"type:varchar(64);index:idx_receipt,unique;not null"`
	Attempt      int    `json:"attempt" gorm:"index:idx_receipt,unique;not null"`
	Party        string `json:"party" gorm:"type:varchar(16);index:idx_receipt,unique;not null"`
	OrderId      string `json:"order_id" gorm:"type:varchar(64);index;not null"`
	InputHash    string `json:"input_hash" gorm:"type:varchar(80)"`
	ResultHash   string `json:"result_hash" gorm:"type:varchar(80)"`
	PayloadHash  string `json:"payload_hash" gorm:"type:varchar(80)"`
	Signature    string `json:"signature" gorm:"type:varchar(256)"`
	SignerDevice string `json:"signer_device" gorm:"type:varchar(64)"`
	CreatedAt    int64  `json:"created_at" gorm:"autoCreateTime"`
}

func (Receipt) TableName() string { return "receipts" }

// SaveReceipt upserts a party's receipt for a task attempt (idempotent on
// re-submit of the same party).
func SaveReceipt(r *Receipt) error {
	var existing Receipt
	err := DB.Where("task_id = ? AND attempt = ? AND party = ?", r.TaskId, r.Attempt, r.Party).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return DB.Create(r).Error
	}
	if err != nil {
		return err
	}
	r.Id = existing.Id
	return DB.Model(&Receipt{}).Where("id = ?", existing.Id).Updates(map[string]any{
		"input_hash": r.InputHash, "result_hash": r.ResultHash,
		"payload_hash": r.PayloadHash, "signature": r.Signature, "signer_device": r.SignerDevice,
	}).Error
}

// GetReceipt returns a specific party's receipt for a task attempt, or nil.
func GetReceipt(taskId string, attempt int, party string) (*Receipt, error) {
	var r Receipt
	err := DB.Where("task_id = ? AND attempt = ? AND party = ?", taskId, attempt, party).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}
