/*
 * @Author: aztec
 * @Date: 2023-08-18 15:02:09
 * @Description: 通知的定义。客户端共享这个文件。注意两边版本要一致
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package antntf

import (
	"fmt"
	"strings"
	"time"

	"github.com/aztecqt/dagger/util"
)

// 通知等级。不同等级的通知有不同的处理方法
type NotifyLevel int

const (
	NotifyLevel_Normal NotifyLevel = iota // 普通通知。00:00~09:00不发，延迟到09:00一起发。其余时间正常发
	NotifyLevel_Error                     // 普通错误通知。任何时间都发。有10分钟内置冷却时间。
	NotifyLevel_Fatal                     // 致命错误通知。必须手动到网页上清除错误标记，不然会一直提醒最新的一条。
)

func notifyLevelToString(level NotifyLevel) string {
	switch level {
	case NotifyLevel_Normal:
		return "普通"
	case NotifyLevel_Error:
		return "错误"
	case NotifyLevel_Fatal:
		return "致命错误"
	default:
		return "unknown"
	}
}

type notify struct {
	Level          NotifyLevel `json:"level"`    // 通知级别
	LocalTimeStamp int64       `json:"local_ts"` // 发送者本地时间戳
	Name           string      `json:"name"`     // 名称。不同发送者的名称不应重复。比如策略名、工具名等。
	Content        string      `json:"content"`  // 消息内容
	ExtraLines     []string    // 额外的内容
	Id             int         // 内部ID
	Resend         int         // 重发次数
}

var accId = 0

func newNotify() *notify {
	n := new(notify)
	n.Id = accId
	accId++
	return n
}

func (n *notify) string(contentMaxLength int) string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("[通知时间]: %s\n", time.UnixMilli(n.LocalTimeStamp).Format(time.DateTime)))
	sb.WriteString(fmt.Sprintf("[通知来源]: %s (id=%d)\n", n.Name, n.Id))
	sb.WriteString(fmt.Sprintf("[通知等级]: %s%s\n", notifyLevelToString(n.Level), util.ValueIf(n.Resend > 0, fmt.Sprintf("(第%d次重发)", n.Resend), "")))
	if contentMaxLength <= 0 || len(n.Content) <= contentMaxLength {
		sb.WriteString(fmt.Sprintf("[通知内容]:\n %s\n", n.Content))
	} else {
		sb.WriteString(fmt.Sprintf("[通知内容]:\n %s...\n", n.Content[:contentMaxLength])) // 截断
	}
	for _, v := range n.ExtraLines {
		sb.WriteString(fmt.Sprintf("%s\n", v))
	}
	return sb.String()
}

func (n *notify) logPrefix() string {
	return fmt.Sprintf("%s ntf(name=%s, id=%d)", logPrefix, n.Name, n.Id)
}
