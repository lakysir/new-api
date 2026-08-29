/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.
*/

package controller

import (
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
)

func GetUserAsyncConcurrencyRules(c *gin.Context) {
	rules, err := model.GetUserAsyncConcurrencyRules()
	if err != nil {
		common.ApiError(c, err)
		return
	}
	userIds := make([]int, 0, len(rules))
	for _, rule := range rules {
		userIds = append(userIds, rule.UserId)
	}
	usernames, err := model.GetUsernamesByIds(userIds)
	if err != nil {
		common.SysError("get usernames for user async concurrency rules error: " + err.Error())
		usernames = map[int]string{}
	}
	for _, rule := range rules {
		rule.Username = usernames[rule.UserId]
		current, countErr := model.CountUnfinishedTaskByUser(rule.UserId)
		if countErr != nil {
			common.SysError("get unfinished task count for user async concurrency rule error: " + countErr.Error())
			continue
		}
		rule.Current = int(current)
	}
	common.ApiSuccess(c, rules)
}

type userAsyncConcurrencyRuleRequest struct {
	UserId         int `json:"user_id"`
	MaxConcurrency int `json:"max_concurrency"`
}

func UpsertUserAsyncConcurrencyRule(c *gin.Context) {
	var req userAsyncConcurrencyRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ApiError(c, err)
		return
	}
	if req.UserId <= 0 {
		common.ApiErrorMsg(c, "无效的用户 ID")
		return
	}
	if req.MaxConcurrency < model.ModelConcurrencyBlocked {
		common.ApiErrorMsg(c, "并发上限只能填 -1（禁止使用）、0（不限制）或正整数")
		return
	}
	if _, err := model.GetUserById(req.UserId, false); err != nil {
		common.ApiErrorMsg(c, "用户不存在")
		return
	}
	rule, err := model.UpsertUserAsyncConcurrencyRule(req.UserId, req.MaxConcurrency)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, rule)
}

func DeleteUserAsyncConcurrencyRule(c *gin.Context) {
	userId, err := strconv.Atoi(c.Param("user_id"))
	if err != nil || userId <= 0 {
		common.ApiErrorMsg(c, "无效的用户 ID")
		return
	}
	if err := model.DeleteUserAsyncConcurrencyRule(userId); err != nil {
		common.ApiError(c, err)
		return
	}
	common.ApiSuccess(c, nil)
}
