package model

import (
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupQuotaApplicationTest(t *testing.T) {
	t.Helper()
	require.NoError(t, DB.AutoMigrate(&QuotaApplication{}, &QuotaAdjustment{}, &ManualInvoiceCredit{}))
	require.NoError(t, DB.Exec("DELETE FROM quota_applications").Error)
	require.NoError(t, DB.Exec("DELETE FROM quota_adjustments").Error)
	require.NoError(t, DB.Exec("DELETE FROM manual_invoice_credits").Error)
	require.NoError(t, DB.Exec("DELETE FROM users").Error)
	t.Cleanup(func() {
		DB.Exec("DELETE FROM quota_applications")
		DB.Exec("DELETE FROM quota_adjustments")
		DB.Exec("DELETE FROM manual_invoice_credits")
		DB.Exec("DELETE FROM users")
	})
}

func createQuotaApplicationTestUsers(t *testing.T) (User, User, User) {
	t.Helper()
	applicant := User{Username: "quota-applicant", Password: "password", Role: common.RoleAdminUser, Status: common.UserStatusEnabled}
	target := User{Username: "quota-target", Password: "password", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Quota: 1000}
	reviewer := User{Username: "quota-reviewer", Password: "password", Role: common.RoleRootUser, Status: common.UserStatusEnabled}
	require.NoError(t, DB.Create(&applicant).Error)
	require.NoError(t, DB.Create(&target).Error)
	require.NoError(t, DB.Create(&reviewer).Error)
	return applicant, target, reviewer
}

func TestReviewQuotaApplicationCreditsExactlyOnce(t *testing.T) {
	setupQuotaApplicationTest(t)
	applicant, target, reviewer := createQuotaApplicationTestUsers(t)
	application := QuotaApplication{
		ApplicantId:        applicant.Id,
		TargetUserId:       target.Id,
		QuotaAmount:        500,
		InvoiceAmountCents: 12000,
		ApplicationInfo:    "Acme payment reference 20260921",
	}
	require.NoError(t, CreateQuotaApplication(&application))

	reviewed, err := ReviewQuotaApplication(application.Id, reviewer.Id, true, "confirmed")
	require.NoError(t, err)
	assert.Equal(t, QuotaApplicationStatusApproved, reviewed.Status)
	assert.Equal(t, 1000, reviewed.QuotaBefore)
	assert.Equal(t, 1500, reviewed.QuotaAfter)

	var updatedTarget User
	require.NoError(t, DB.First(&updatedTarget, target.Id).Error)
	assert.Equal(t, 1500, updatedTarget.Quota)

	var adjustmentCount int64
	require.NoError(t, DB.Model(&QuotaAdjustment{}).Where("user_id = ?", target.Id).Count(&adjustmentCount).Error)
	assert.Equal(t, int64(1), adjustmentCount)
	var creditCount int64
	require.NoError(t, DB.Model(&ManualInvoiceCredit{}).Where("user_id = ?", target.Id).Count(&creditCount).Error)
	assert.Equal(t, int64(1), creditCount)

	_, err = ReviewQuotaApplication(application.Id, reviewer.Id, true, "retry")
	require.Error(t, err)
	require.NoError(t, DB.First(&updatedTarget, target.Id).Error)
	assert.Equal(t, 1500, updatedTarget.Quota)
}

func TestReviewQuotaApplicationRejectsWithoutCrediting(t *testing.T) {
	setupQuotaApplicationTest(t)
	applicant, target, reviewer := createQuotaApplicationTestUsers(t)
	application := QuotaApplication{
		ApplicantId:        applicant.Id,
		TargetUserId:       target.Id,
		QuotaAmount:        500,
		InvoiceAmountCents: 12000,
		ApplicationInfo:    "Missing payment proof",
	}
	require.NoError(t, CreateQuotaApplication(&application))

	_, err := ReviewQuotaApplication(application.Id, reviewer.Id, false, "")
	require.Error(t, err)
	reviewed, err := ReviewQuotaApplication(application.Id, reviewer.Id, false, "payment not received")
	require.NoError(t, err)
	assert.Equal(t, QuotaApplicationStatusRejected, reviewed.Status)
	assert.Equal(t, "payment not received", reviewed.ReviewComment)

	var updatedTarget User
	require.NoError(t, DB.First(&updatedTarget, target.Id).Error)
	assert.Equal(t, 1000, updatedTarget.Quota)
}
