package model

import "gorm.io/gorm"

// UserAsset stores metadata for a user-owned media object in external storage.
// File bytes never enter the primary database.
type UserAsset struct {
	Id          int    `json:"id" gorm:"primaryKey;autoIncrement"`
	UserId      int    `json:"-" gorm:"not null;index:idx_user_assets_user_created"`
	ObjectKey   string `json:"-" gorm:"type:varchar(512);not null;uniqueIndex"`
	Filename    string `json:"filename" gorm:"type:varchar(255);not null"`
	ContentType string `json:"content_type" gorm:"type:varchar(127);not null"`
	MediaType   string `json:"media_type" gorm:"type:varchar(16);not null"`
	Size        int64  `json:"size" gorm:"not null"`
	PublicUrl   string `json:"url" gorm:"type:varchar(1024);not null"`
	CreatedAt   int64  `json:"created_at" gorm:"autoCreateTime;index:idx_user_assets_user_created,sort:desc"`
}

func (UserAsset) TableName() string { return "user_assets" }

func CreateUserAsset(asset *UserAsset) error {
	return DB.Create(asset).Error
}

func ListUserAssets(userId int) ([]UserAsset, error) {
	var assets []UserAsset
	err := DB.Where("user_id = ?", userId).Order("created_at desc, id desc").Find(&assets).Error
	return assets, err
}

func GetUserAsset(userId, id int) (*UserAsset, error) {
	var asset UserAsset
	if err := DB.Where("user_id = ? AND id = ?", userId, id).First(&asset).Error; err != nil {
		return nil, err
	}
	return &asset, nil
}

func DeleteUserAsset(userId, id int) error {
	result := DB.Where("user_id = ? AND id = ?", userId, id).Delete(&UserAsset{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
