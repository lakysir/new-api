package model

import (
	"errors"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	QuotaApplicationStatusPending  = "pending"
	QuotaApplicationStatusApproved = "approved"
	QuotaApplicationStatusRejected = "rejected"
)

type QuotaApplication struct {
	Id                 int    `json:"id"`
	ApplicantId        int    `json:"applicant_id" gorm:"index;not null"`
	ApplicantUsername  string `json:"applicant_username" gorm:"->;-:migration"`
	TargetUserId       int    `json:"target_user_id" gorm:"index;not null"`
	TargetUsername     string `json:"target_username" gorm:"->;-:migration"`
	TargetDisplayName  string `json:"target_display_name" gorm:"->;-:migration"`
	QuotaAmount        int    `json:"quota_amount" gorm:"not null"`
	InvoiceAmountCents int64  `json:"invoice_amount_cents" gorm:"not null"`
	ApplicationInfo    string `json:"application_info" gorm:"type:text;not null"`
	Status             string `json:"status" gorm:"type:varchar(16);index;not null"`
	ReviewedBy         int    `json:"reviewed_by" gorm:"index"`
	ReviewerUsername   string `json:"reviewer_username" gorm:"->;-:migration"`
	ReviewComment      string `json:"review_comment" gorm:"type:varchar(500)"`
	QuotaBefore        int    `json:"quota_before"`
	QuotaAfter         int    `json:"quota_after"`
	CreatedAt          int64  `json:"created_at" gorm:"autoCreateTime;index"`
	ReviewedAt         int64  `json:"reviewed_at"`
	UpdatedAt          int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type QuotaApplicationTarget struct {
	Id          int    `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Quota       int    `json:"quota"`
}

func IsQuotaApplicationStatus(status string) bool {
	return status == QuotaApplicationStatusPending || status == QuotaApplicationStatusApproved || status == QuotaApplicationStatusRejected
}

func CreateQuotaApplication(application *QuotaApplication) error {
	application.ApplicationInfo = strings.TrimSpace(application.ApplicationInfo)
	if application.QuotaAmount <= 0 {
		return errors.New("quota amount must be positive")
	}
	if application.InvoiceAmountCents < 0 {
		return errors.New("invoiceable CNY amount cannot be negative")
	}
	if application.ApplicationInfo == "" || len(application.ApplicationInfo) > 2000 {
		return errors.New("application information is required and must not exceed 2000 characters")
	}

	var target User
	if err := DB.Select("id", "role", "status").Where("id = ?", application.TargetUserId).First(&target).Error; err != nil {
		return errors.New("target user not found")
	}
	if target.Role != common.RoleCommonUser || target.Status != common.UserStatusEnabled {
		return errors.New("target must be an enabled normal user")
	}

	application.Status = QuotaApplicationStatusPending
	application.ReviewedBy = 0
	application.ReviewedAt = 0
	application.QuotaBefore = 0
	application.QuotaAfter = 0
	return DB.Create(application).Error
}

func SearchQuotaApplicationTargets(keyword string, limit int) ([]QuotaApplicationTarget, error) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return []QuotaApplicationTarget{}, nil
	}
	if limit <= 0 || limit > 20 {
		limit = 20
	}
	like := "%" + keyword + "%"
	query := DB.Model(&User{}).
		Select("id", "username", "display_name", "quota").
		Where("role = ? AND status = ?", common.RoleCommonUser, common.UserStatusEnabled).
		Where("username LIKE ? OR display_name LIKE ?", like, like)
	if id, err := strconv.Atoi(keyword); err == nil {
		query = DB.Model(&User{}).
			Select("id", "username", "display_name", "quota").
			Where("role = ? AND status = ?", common.RoleCommonUser, common.UserStatusEnabled).
			Where("id = ? OR username LIKE ? OR display_name LIKE ?", id, like, like)
	}
	var users []QuotaApplicationTarget
	err := query.Order("id DESC").Limit(limit).Scan(&users).Error
	return users, err
}

func ListQuotaApplications(applicantId *int, status, keyword string, offset, limit int) ([]QuotaApplication, int64, error) {
	query := DB.Table("quota_applications AS qa").
		Joins("LEFT JOIN users AS applicant ON applicant.id = qa.applicant_id").
		Joins("LEFT JOIN users AS target ON target.id = qa.target_user_id").
		Joins("LEFT JOIN users AS reviewer ON reviewer.id = qa.reviewed_by")
	if applicantId != nil {
		query = query.Where("qa.applicant_id = ?", *applicantId)
	}
	if status != "" {
		query = query.Where("qa.status = ?", status)
	}
	if keyword = strings.TrimSpace(keyword); keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("applicant.username LIKE ? OR target.username LIKE ? OR target.display_name LIKE ?", like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var applications []QuotaApplication
	err := query.Select("qa.*, applicant.username AS applicant_username, target.username AS target_username, target.display_name AS target_display_name, reviewer.username AS reviewer_username").
		Order("qa.id DESC").Offset(offset).Limit(limit).Scan(&applications).Error
	return applications, total, err
}

func GetQuotaApplication(id int, applicantId *int) (*QuotaApplication, error) {
	query := DB.Table("quota_applications AS qa").
		Select("qa.*, applicant.username AS applicant_username, target.username AS target_username, target.display_name AS target_display_name, reviewer.username AS reviewer_username").
		Joins("LEFT JOIN users AS applicant ON applicant.id = qa.applicant_id").
		Joins("LEFT JOIN users AS target ON target.id = qa.target_user_id").
		Joins("LEFT JOIN users AS reviewer ON reviewer.id = qa.reviewed_by").
		Where("qa.id = ?", id)
	if applicantId != nil {
		query = query.Where("qa.applicant_id = ?", *applicantId)
	}
	var application QuotaApplication
	if err := query.Scan(&application).Error; err != nil {
		return nil, err
	}
	if application.Id == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &application, nil
}

func ReviewQuotaApplication(id, reviewerId int, approved bool, comment string) (*QuotaApplication, error) {
	comment = strings.TrimSpace(comment)
	if !approved && comment == "" {
		return nil, errors.New("rejection reason is required")
	}
	if len(comment) > 500 {
		return nil, errors.New("review comment must not exceed 500 characters")
	}

	var reviewed QuotaApplication
	err := DB.Transaction(func(tx *gorm.DB) error {
		var application QuotaApplication
		if err := tx.Where("id = ?", id).First(&application).Error; err != nil {
			return err
		}
		if application.Status != QuotaApplicationStatusPending {
			return errors.New("only pending applications can be reviewed")
		}

		now := common.GetTimestamp()
		updates := map[string]interface{}{
			"reviewed_by":    reviewerId,
			"reviewed_at":    now,
			"review_comment": comment,
		}
		if approved {
			var target User
			if err := tx.Select("id", "role", "status").Where("id = ?", application.TargetUserId).First(&target).Error; err != nil {
				return errors.New("target user not found")
			}
			if target.Role != common.RoleCommonUser || target.Status != common.UserStatusEnabled {
				return errors.New("target must be an enabled normal user")
			}
			before, after, err := adjustUserQuotaWithManualInvoiceCreditTx(tx, application.TargetUserId, reviewerId, "add", application.QuotaAmount, application.InvoiceAmountCents)
			if err != nil {
				return err
			}
			updates["status"] = QuotaApplicationStatusApproved
			updates["quota_before"] = before
			updates["quota_after"] = after
		} else {
			updates["status"] = QuotaApplicationStatusRejected
		}

		result := tx.Model(&QuotaApplication{}).Where("id = ? AND status = ?", id, QuotaApplicationStatusPending).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("application has already been reviewed")
		}
		return tx.First(&reviewed, id).Error
	})
	return &reviewed, err
}
