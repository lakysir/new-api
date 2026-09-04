package model

import "github.com/QuantumNous/new-api/common"

// QuotaAdjustment records a quota change made by an administrator.
type QuotaAdjustment struct {
	Id          int   `json:"id"`
	UserId      int   `json:"user_id" gorm:"index;not null"`
	AdminId     int   `json:"-" gorm:"index;not null"`
	Mode        string `json:"mode" gorm:"type:varchar(16);not null"`
	Delta       int   `json:"quota_delta" gorm:"not null"`
	QuotaBefore int   `json:"quota_before" gorm:"not null"`
	QuotaAfter  int   `json:"quota_after" gorm:"not null"`
	CreatedAt   int64 `json:"create_time" gorm:"index;not null"`
}

func (QuotaAdjustment) TableName() string { return "quota_adjustments" }

func GetUserQuotaAdjustments(userId int) ([]*QuotaAdjustment, error) {
	var items []*QuotaAdjustment
	err := DB.Where("user_id = ?", userId).Order("created_at DESC, id DESC").Find(&items).Error
	return items, err
}

func newQuotaAdjustment(userId, adminId int, mode string, before, after int) *QuotaAdjustment {
	return &QuotaAdjustment{UserId: userId, AdminId: adminId, Mode: mode, Delta: after - before, QuotaBefore: before, QuotaAfter: after, CreatedAt: common.GetTimestamp()}
}
