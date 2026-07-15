package model

import (
	"errors"
	"math"
	"net/mail"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

const (
	InvoiceStatusPending   = "pending"
	InvoiceStatusApproved  = "approved"
	InvoiceStatusSent      = "sent"
	InvoiceStatusRejected  = "rejected"
	InvoiceStatusCancelled = "cancelled"
	MinimumInvoiceCents    = int64(100000)
)

type InvoiceApplication struct {
	Id             int    `json:"id"`
	UserId         int    `json:"user_id" gorm:"index;not null"`
	Username       string `json:"username" gorm:"-:all"`
	InvoiceType    string `json:"invoice_type" gorm:"type:varchar(16);not null"`
	ProfileId      int    `json:"profile_id" gorm:"index"`
	Title          string `json:"title" gorm:"type:varchar(200);not null"`
	TaxNumber      string `json:"tax_number" gorm:"type:varchar(64)"`
	RegisteredAddress string `json:"registered_address" gorm:"type:varchar(255)"`
	RegisteredPhone   string `json:"registered_phone" gorm:"type:varchar(64)"`
	BankName          string `json:"bank_name" gorm:"type:varchar(128)"`
	BankAccount       string `json:"bank_account" gorm:"type:varchar(64)"`
	Email          string `json:"email" gorm:"type:varchar(128);not null"`
	AmountCents    int64  `json:"amount_cents" gorm:"not null"`
	Remark         string `json:"remark" gorm:"type:varchar(500)"`
	Status         string `json:"status" gorm:"type:varchar(16);index;not null"`
	RejectReason   string `json:"reject_reason" gorm:"type:varchar(500)"`
	ReviewedBy     int    `json:"reviewed_by"`
	ReviewedAt     int64  `json:"reviewed_at"`
	SentBy         int    `json:"sent_by"`
	SentAt         int64  `json:"sent_at"`
	CreatedAt      int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt      int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

type InvoiceProfile struct {
	Id                int    `json:"id"`
	UserId            int    `json:"user_id" gorm:"index;not null"`
	Title             string `json:"title" gorm:"type:varchar(200);not null"`
	TaxNumber         string `json:"tax_number" gorm:"type:varchar(64);not null"`
	RegisteredAddress string `json:"registered_address" gorm:"type:varchar(255)"`
	RegisteredPhone   string `json:"registered_phone" gorm:"type:varchar(64)"`
	BankName          string `json:"bank_name" gorm:"type:varchar(128)"`
	BankAccount       string `json:"bank_account" gorm:"type:varchar(64)"`
	Email             string `json:"email" gorm:"type:varchar(128);not null"`
	IsDefault         bool   `json:"is_default" gorm:"column:is_default"`
	CreatedAt         int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt         int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func normalizeInvoiceProfile(profile *InvoiceProfile) error {
	profile.Title = strings.TrimSpace(profile.Title)
	profile.TaxNumber = strings.TrimSpace(profile.TaxNumber)
	profile.RegisteredAddress = strings.TrimSpace(profile.RegisteredAddress)
	profile.RegisteredPhone = strings.TrimSpace(profile.RegisteredPhone)
	profile.BankName = strings.TrimSpace(profile.BankName)
	profile.BankAccount = strings.TrimSpace(profile.BankAccount)
	profile.Email = strings.TrimSpace(profile.Email)
	if profile.Title == "" || profile.TaxNumber == "" || profile.Email == "" {
		return errors.New("enterprise title, tax number and email are required")
	}
	if len(profile.Title) > 200 || len(profile.TaxNumber) > 64 || len(profile.RegisteredAddress) > 255 || len(profile.RegisteredPhone) > 64 || len(profile.BankName) > 128 || len(profile.BankAccount) > 64 || len(profile.Email) > 128 {
		return errors.New("invoice profile information is too long")
	}
	if _, err := mail.ParseAddress(profile.Email); err != nil {
		return errors.New("invalid delivery email")
	}
	return nil
}

func SaveInvoiceProfile(userId int, profile *InvoiceProfile) error {
	if err := normalizeInvoiceProfile(profile); err != nil { return err }
	return DB.Transaction(func(tx *gorm.DB) error {
		if profile.IsDefault {
			if err := tx.Model(&InvoiceProfile{}).Where("user_id = ?", userId).Update("is_default", false).Error; err != nil { return err }
		}
		profile.UserId = userId
		if profile.Id == 0 { return tx.Create(profile).Error }
		result := tx.Model(&InvoiceProfile{}).Where("id = ? AND user_id = ?", profile.Id, userId).Updates(map[string]interface{}{
			"title": profile.Title, "tax_number": profile.TaxNumber, "registered_address": profile.RegisteredAddress,
			"registered_phone": profile.RegisteredPhone, "bank_name": profile.BankName, "bank_account": profile.BankAccount,
			"email": profile.Email, "is_default": profile.IsDefault,
		})
		if result.Error != nil { return result.Error }
		if result.RowsAffected != 1 { return errors.New("invoice profile not found") }
		return tx.Where("id = ? AND user_id = ?", profile.Id, userId).First(profile).Error
	})
}

func ListInvoiceProfiles(userId int) ([]*InvoiceProfile, error) {
	var profiles []*InvoiceProfile
	err := DB.Where("user_id = ?", userId).Order("is_default DESC, id DESC").Find(&profiles).Error
	return profiles, err
}

func DeleteInvoiceProfile(userId, id int) error {
	result := DB.Where("id = ? AND user_id = ?", id, userId).Delete(&InvoiceProfile{})
	if result.Error != nil { return result.Error }
	if result.RowsAffected != 1 { return errors.New("invoice profile not found") }
	return nil
}

type ManualInvoiceCredit struct {
	Id          int   `json:"id"`
	UserId      int   `json:"user_id" gorm:"index;not null"`
	AdminId     int   `json:"admin_id" gorm:"index;not null"`
	QuotaAmount int   `json:"quota_amount" gorm:"not null"`
	AmountCents int64 `json:"amount_cents" gorm:"not null"`
	CreatedAt   int64 `json:"created_at" gorm:"autoCreateTime"`
}

type InvoiceOverview struct {
	Enabled        bool                  `json:"enabled"`
	PaidCents      int64                 `json:"paid_cents"`
	OccupiedCents  int64                 `json:"occupied_cents"`
	AvailableCents int64                 `json:"available_cents"`
	Applications   []*InvoiceApplication `json:"applications"`
}

func paidTopUpCents(tx *gorm.DB, userId int) (int64, error) {
	var topups []TopUp
	if err := tx.Select("money").Where("user_id = ? AND status = ?", userId, common.TopUpStatusSuccess).Find(&topups).Error; err != nil {
		return 0, err
	}
	var total int64
	for _, topup := range topups {
		total += int64(math.Round(topup.Money * 100))
	}
	var manualTotal int64
	if err := tx.Model(&ManualInvoiceCredit{}).Where("user_id = ?", userId).Select("COALESCE(SUM(amount_cents), 0)").Scan(&manualTotal).Error; err != nil {
		return 0, err
	}
	total += manualTotal
	return total, nil
}

func AdjustUserQuotaWithManualInvoiceCredit(userId, adminId int, mode string, value int, invoiceAmountCents int64) (int, error) {
	newQuota := 0
	err := DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&user, userId).Error; err != nil {
			return err
		}

		positiveDelta := 0
		switch mode {
		case "add":
			if value <= 0 {
				return errors.New("quota adjustment must be positive")
			}
			positiveDelta = value
			newQuota = user.Quota + value
		case "subtract":
			if value <= 0 {
				return errors.New("quota adjustment must be positive")
			}
			newQuota = user.Quota - value
		case "override":
			newQuota = value
			if value > user.Quota {
				positiveDelta = value - user.Quota
			}
		default:
			return errors.New("invalid quota adjustment mode")
		}

		if err := tx.Model(&User{}).Where("id = ?", userId).Update("quota", newQuota).Error; err != nil {
			return err
		}
		if positiveDelta == 0 {
			return nil
		}
		if invoiceAmountCents <= 0 {
			return errors.New("invoiceable CNY amount is required for a positive quota adjustment")
		}
		credit := ManualInvoiceCredit{
			UserId:      userId,
			AdminId:     adminId,
			QuotaAmount: positiveDelta,
			AmountCents: invoiceAmountCents,
		}
		return tx.Create(&credit).Error
	})
	return newQuota, err
}

func invoiceOccupiedCents(tx *gorm.DB, userId int) (int64, error) {
	var applications []InvoiceApplication
	if err := tx.Select("amount_cents").Where("user_id = ? AND status NOT IN ?", userId, []string{InvoiceStatusCancelled, InvoiceStatusRejected}).Find(&applications).Error; err != nil {
		return 0, err
	}
	var total int64
	for _, application := range applications {
		total += application.AmountCents
	}
	return total, nil
}

func GetInvoiceOverview(userId int) (*InvoiceOverview, error) {
	var user User
	if err := DB.Select("invoice_enabled").First(&user, userId).Error; err != nil {
		return nil, err
	}
	paid, err := paidTopUpCents(DB, userId)
	if err != nil {
		return nil, err
	}
	occupied, err := invoiceOccupiedCents(DB, userId)
	if err != nil {
		return nil, err
	}
	var applications []*InvoiceApplication
	if err := DB.Where("user_id = ?", userId).Order("id DESC").Find(&applications).Error; err != nil {
		return nil, err
	}
	available := paid - occupied
	if available < 0 {
		available = 0
	}
	return &InvoiceOverview{user.InvoiceEnabled, paid, occupied, available, applications}, nil
}

func CreateInvoiceApplication(userId int, application *InvoiceApplication) error {
	return DB.Transaction(func(tx *gorm.DB) error {
		var user User
		if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&user, userId).Error; err != nil {
			return err
		}
		if !user.InvoiceEnabled {
			return errors.New("invoice application is not enabled for this account")
		}
		if application.AmountCents < MinimumInvoiceCents {
			return errors.New("invoice amount must be at least CNY 1,000")
		}
		application.InvoiceType = strings.TrimSpace(application.InvoiceType)
		application.Title = strings.TrimSpace(application.Title)
		application.TaxNumber = strings.TrimSpace(application.TaxNumber)
		application.Email = strings.TrimSpace(application.Email)
		if application.InvoiceType == "enterprise" {
			if application.ProfileId == 0 { return errors.New("select a saved enterprise invoice profile") }
			var profile InvoiceProfile
			if err := tx.Where("id = ? AND user_id = ?", application.ProfileId, userId).First(&profile).Error; err != nil { return errors.New("invoice profile not found") }
			application.Title = profile.Title
			application.TaxNumber = profile.TaxNumber
			application.RegisteredAddress = profile.RegisteredAddress
			application.RegisteredPhone = profile.RegisteredPhone
			application.BankName = profile.BankName
			application.BankAccount = profile.BankAccount
			application.Email = profile.Email
		}
		if application.Title == "" || application.Email == "" || (application.InvoiceType != "personal" && application.InvoiceType != "enterprise") {
			return errors.New("invalid invoice information")
		}
		if _, err := mail.ParseAddress(application.Email); err != nil {
			return errors.New("invalid delivery email")
		}
		if application.InvoiceType == "enterprise" && application.TaxNumber == "" {
			return errors.New("tax number is required for enterprise invoices")
		}
		paid, err := paidTopUpCents(tx, userId)
		if err != nil { return err }
		occupied, err := invoiceOccupiedCents(tx, userId)
		if err != nil { return err }
		if application.AmountCents > paid-occupied {
			return errors.New("invoice amount exceeds available paid amount")
		}
		application.UserId, application.Status = userId, InvoiceStatusPending
		return tx.Create(application).Error
	})
}

func CancelInvoiceApplication(userId, id int) error {
	result := DB.Model(&InvoiceApplication{}).Where("id = ? AND user_id = ? AND status IN ?", id, userId, []string{InvoiceStatusPending, InvoiceStatusRejected}).Update("status", InvoiceStatusCancelled)
	if result.Error != nil { return result.Error }
	if result.RowsAffected != 1 { return errors.New("only pending or rejected applications can be cancelled") }
	return nil
}

func ListInvoiceApplications(status string) ([]*InvoiceApplication, error) {
	query := DB.Table("invoice_applications AS i").Select("i.*, u.username").Joins("LEFT JOIN users AS u ON u.id = i.user_id").Order("i.id DESC")
	if status != "" { query = query.Where("i.status = ?", status) }
	var items []*InvoiceApplication
	if err := query.Scan(&items).Error; err != nil { return nil, err }
	return items, nil
}

func ReviewInvoiceApplication(id, adminId int, approve bool, reason string) error {
	updates := map[string]interface{}{"reviewed_by": adminId, "reviewed_at": common.GetTimestamp()}
	if approve { updates["status"] = InvoiceStatusApproved; updates["reject_reason"] = "" } else { if strings.TrimSpace(reason) == "" { return errors.New("rejection reason is required") }; updates["status"] = InvoiceStatusRejected; updates["reject_reason"] = strings.TrimSpace(reason) }
	result := DB.Model(&InvoiceApplication{}).Where("id = ? AND status = ?", id, InvoiceStatusPending).Updates(updates)
	if result.Error != nil { return result.Error }; if result.RowsAffected != 1 { return errors.New("only pending applications can be reviewed") }; return nil
}

func MarkInvoiceSent(id, adminId int) error {
	result := DB.Model(&InvoiceApplication{}).Where("id = ? AND status = ?", id, InvoiceStatusApproved).Updates(map[string]interface{}{"status": InvoiceStatusSent, "sent_by": adminId, "sent_at": common.GetTimestamp()})
	if result.Error != nil { return result.Error }; if result.RowsAffected != 1 { return errors.New("only approved applications can be marked as sent") }; return nil
}
