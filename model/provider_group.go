package model

import (
	"errors"
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"gorm.io/gorm"
)

// ProviderGroup is the logical grouping a provider's nodes belong to. Every user
// gets exactly one group, auto-created from their username the first time they
// open the node console (or register a node). The group name equals the username
// and is not editable; the Id is stable and used by clients to filter offers to
// a single provider (architecture: node grouping / offer filtering).
type ProviderGroup struct {
	Id        string `json:"id" gorm:"primaryKey;type:varchar(64)"`
	UserId    int    `json:"user_id" gorm:"uniqueIndex;not null"`
	Name      string `json:"name" gorm:"type:varchar(64);index"`
	CreatedAt int64  `json:"created_at" gorm:"autoCreateTime"`
	UpdatedAt int64  `json:"updated_at" gorm:"autoUpdateTime"`
}

func (ProviderGroup) TableName() string { return "provider_groups" }

// providerGroupName derives the group display name from a username. Falls back
// to "provider-<userId>" when the username is empty so the group always has a
// human-readable label.
func providerGroupName(userId int, username string) string {
	name := strings.TrimSpace(username)
	if name == "" {
		return fmt.Sprintf("provider-%d", userId)
	}
	return name
}

// GetOrCreateProviderGroup returns the caller's provider group, creating it on
// first use. The group id is a stable "pg_"-prefixed UUID; the name is the
// user's username. Re-entry returns the existing row (and refreshes the name if
// the username changed) so all of a user's nodes share one group.
func GetOrCreateProviderGroup(userId int, username string) (*ProviderGroup, error) {
	if userId <= 0 {
		return nil, errors.New("invalid user id")
	}
	name := providerGroupName(userId, username)
	var group ProviderGroup
	err := DB.Where("user_id = ?", userId).First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		group = ProviderGroup{Id: "pg_" + common.GetUUID(), UserId: userId, Name: name}
		if createErr := DB.Create(&group).Error; createErr != nil {
			// Lost a create race: re-read the row the winner inserted.
			if isUniqueConstraintErr(createErr) {
				if reErr := DB.Where("user_id = ?", userId).First(&group).Error; reErr == nil {
					return &group, nil
				}
			}
			return nil, createErr
		}
		return &group, nil
	}
	if err != nil {
		return nil, err
	}
	// Keep the display name in sync with the current username.
	if group.Name != name {
		DB.Model(&ProviderGroup{}).Where("id = ?", group.Id).Update("name", name)
		group.Name = name
	}
	return &group, nil
}

// GetProviderGroupByUser returns a user's group without creating it, or nil.
func GetProviderGroupByUser(userId int) (*ProviderGroup, error) {
	var group ProviderGroup
	err := DB.Where("user_id = ?", userId).First(&group).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &group, nil
}

// SearchProviderGroups returns groups whose name contains the query (case
// sensitivity follows the database collation). An empty query returns nothing so
// clients don't accidentally pull the full table; capped at 20 rows.
func SearchProviderGroups(query string) ([]ProviderGroup, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return []ProviderGroup{}, nil
	}
	var groups []ProviderGroup
	err := DB.Where("name LIKE ?", "%"+q+"%").Order("name asc").Limit(20).Find(&groups).Error
	return groups, err
}

// AssignNodesToProviderGroup backfills a user's nodes that have no group yet,
// pointing them at the user's group. Called when the group is resolved so
// pre-existing nodes join it without a manual migration.
func AssignNodesToProviderGroup(userId int, groupId string) error {
	if groupId == "" {
		return nil
	}
	return DB.Model(&Node{}).
		Where("user_id = ? AND (provider_group_id = '' OR provider_group_id IS NULL)", userId).
		Update("provider_group_id", groupId).Error
}

// ensureUserProviderGroup resolves (and lazily creates) the group id for a user,
// used when registering a node so it is grouped from the first heartbeat.
func ensureUserProviderGroup(userId int) string {
	username, _ := GetUsernameById(userId, false)
	group, err := GetOrCreateProviderGroup(userId, username)
	if err != nil || group == nil {
		return ""
	}
	return group.Id
}
