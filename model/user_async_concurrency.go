/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

package model

import "github.com/QuantumNous/new-api/common"

// UserAsyncConcurrency limits the total number of unfinished async tasks for a user.
// MaxConcurrency == 0 means unlimited; -1 blocks all async task submissions.
type UserAsyncConcurrency struct {
	Id             int    `json:"id"`
	UserId         int    `json:"user_id" gorm:"not null;uniqueIndex"`
	MaxConcurrency int    `json:"max_concurrency" gorm:"not null;default:0"`
	CreatedTime    int64  `json:"created_time" gorm:"bigint"`
	UpdatedTime    int64  `json:"updated_time" gorm:"bigint"`
	Username       string `json:"username,omitempty" gorm:"-"`
	Current        int    `json:"current" gorm:"-"`
}

func GetUserAsyncConcurrencyLimit(userId int) int {
	if userId <= 0 {
		return 0
	}
	var rule UserAsyncConcurrency
	if err := DB.Where("user_id = ?", userId).First(&rule).Error; err != nil {
		return 0
	}
	return rule.MaxConcurrency
}

func GetUserAsyncConcurrencyRules() ([]*UserAsyncConcurrency, error) {
	var rules []*UserAsyncConcurrency
	if err := DB.Order("user_id asc").Find(&rules).Error; err != nil {
		return nil, err
	}
	return rules, nil
}

func UpsertUserAsyncConcurrencyRule(userId int, maxConcurrency int) (*UserAsyncConcurrency, error) {
	now := common.GetTimestamp()
	var rule UserAsyncConcurrency
	err := DB.Where("user_id = ?", userId).First(&rule).Error
	if err == nil {
		rule.MaxConcurrency = maxConcurrency
		rule.UpdatedTime = now
		if err := DB.Save(&rule).Error; err != nil {
			return nil, err
		}
		return &rule, nil
	}
	rule = UserAsyncConcurrency{UserId: userId, MaxConcurrency: maxConcurrency, CreatedTime: now, UpdatedTime: now}
	if err := DB.Create(&rule).Error; err != nil {
		return nil, err
	}
	return &rule, nil
}

func DeleteUserAsyncConcurrencyRule(userId int) error {
	return DB.Where("user_id = ?", userId).Delete(&UserAsyncConcurrency{}).Error
}
