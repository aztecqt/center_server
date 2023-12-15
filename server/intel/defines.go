/*
 * @Author: aztec
 * @Date: 2023-01-09 16:38:22
 * @Description:
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */

package intel

import (
	"time"
)

// 某种collector可以输出的情报类型
type IntelMenu struct {
	Type                   string         `json:"type"`
	SubTypes               map[string]int `json:"subtypes"`
	SubtypeUncertain       bool           `json:"subtype_uncertain"`        // 不特定的Subtype类型
	SybTypeUncertainReason string         `json:"subtype_uncertain_reason"` // 不特定的理由
}

const (
	DingType_Text = "txt"
	DingType_Link = "link"
)

// 一条捕获的情报
type Intel struct {
	Seq      int       `json:"seq"`       // 一个递增的ID，主要用于客户端识别新旧消息
	Time     time.Time `json:"time"`      // 情报发生的时间
	Level    int       `json:"level"`     // 消息等级。0=调试消息，1=正式消息
	Type     string    `json:"type"`      // 必填，主类型。用于用户订阅筛选
	SubType  string    `json:"subtype"`   // 选填，次要类型。用于用户订阅筛选
	DingType string    `json:"ding_type"` // txt/link表示文字/链接消息。空表示不发给dingding
	Title    string    `json:"title"`     // 可选
	Content  string    `json:"content"`   // 必选
	TTS      string    `json:"tts"`       // 可选，需要语音播报的内容
	Url      string    `json:"url"`       // 可选
}
