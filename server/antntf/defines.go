/*
 * @Author: aztec
 * @Date: 2023-08-17 17:27:34
 * @Description:
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package antntf

import "strings"

var logPrefix = "ant-notify"
var specName_System = "System"

const (
	RedisKey_StratergyConfig = "strategy_config"
	RedisKey_DingdingConfig  = "dingding_cfg"
	RedisKey_Error           = "strategy_error"
)

// 钉钉启动配置
type dingLaunchConfig struct {
	AgentId string `json:"agentId"`
	Key     string `json:"key"`
	Secret  string `json:"secret"`
}

// 钉钉人员配置
type dingPersonConfig struct {
	Name    string `json:"name"`
	Mob     string `json:"mob"`
	IsAdmin string `json:"forever"`
}

// 策略配置
type stratergyConfig struct {
	DingUserStr string `json:"dingding_users"`
	DingUsers   []string
}

func (c *stratergyConfig) parse() {
	c.DingUsers = strings.Split(c.DingUserStr, ",")
}
