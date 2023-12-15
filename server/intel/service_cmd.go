/*
 * @Author: aztec
 * @Date: 2023-01-09 16:38:22
 * @Description: intel service的Command处理
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package intel

import (
	"fmt"
	"strings"

	"github.com/aztecqt/dagger/util"
)

func (s *Service) OnCommand(cmdLine string, onResp func(string)) {
	splited := strings.Split(cmdLine, " ")
	var op, uid, nick string

	if len(splited) < 3 {
		onResp(fmt.Sprintf("internal error"))
		return
	} else {
		op = splited[0]
		uid = splited[len(splited)-2]
		nick = splited[len(splited)-1]
		splited = splited[:len(splited)-2]
	}

	switch op {
	case "help":
		s.onCmdHelp(onResp)
	case "test":
		s.onCmdTest(onResp)
	case "ls":
		s.onCmdLs(splited, onResp)
	case "my":
		s.onCmdMy(uid, nick, onResp)
	case "s":
		s.onCmdSubscribe(splited, uid, nick, onResp)
	case "us":
		s.onCmdUnsubscribe(splited, uid, nick, onResp)
	case "ss":
		s.onCmdSubscribeSub(splited, uid, nick, onResp)
	case "uss":
		s.onCmdUnsubscribeSub(splited, uid, nick, onResp)
	case "xs":
		s.onCmdExcludeSub(splited, uid, nick, onResp)
	case "uxs":
		s.onCmdUnexcludeSub(splited, uid, nick, onResp)
	case "css":
		s.onCmdClearSubchannelSettings(splited, uid, nick, onResp)
	default:
		onResp(fmt.Sprintf("unknown command: `%s`", op))
	}
}

func (c *Service) onCmdHelp(onResp func(string)) {
	sb := strings.Builder{}
	sb.WriteString("情报订阅命令格式：\n")
	sb.WriteString("ls (查看所有可订阅的频道)\n")
	sb.WriteString("ls <chName> (查看频道下的子频道)\n")
	sb.WriteString("my (查看自己已经订阅的频道)\n")
	sb.WriteString("s <chName> (subscribe, 订阅某频道)\n")
	sb.WriteString("us <chName> (unsubscribe, 取消订阅某频道)\n")
	sb.WriteString("ss <chName> <sub_chName> (subscribe-subchannel, 订阅子频道。不订阅任何子频道等同于订阅所有子频道)\n")
	sb.WriteString("uss <chName> <sub_chName> (unsubscribe-subchannel, 取消订阅子频道)\n")
	sb.WriteString("xs <chName> <sub_chName> (exclude-subchannel, 排除子频道)\n")
	sb.WriteString("uxs <chName> <sub_chName> (unexclude-subchannel,取消排除子频道)\n")
	sb.WriteString("css <chName> (clear-subchannel-settings, 清空某频道下的子频道设置，回到默认全部接收的状态)\n")
	onResp(sb.String())
}

func (c *Service) onCmdTest(onResp func(string)) {
	// 发送两个测试情报
	i1 := Intel{}
	i1.Level = 1
	i1.Type = "test"
	i1.SubType = "foo"
	i1.Title = "test-intel"
	i1.Content = "fooooooooooooooooo"

	i2 := Intel{}
	i2.Level = 1
	i2.Type = "test"
	i2.SubType = "bar"
	i2.Title = "test-intel"
	i2.Content = "barrrrrrrrrrrrrrrr"

	c.processIntel(i1)
	c.processIntel(i2)
	onResp("test intel sended")
}

func (c *Service) onCmdLs(splited []string, onResp func(string)) {
	if len(splited) < 2 {
		// 展示所有频道名
		sb := strings.Builder{}
		sb.WriteString("当前可订阅的channel：\n")
		for i, mainType := range c.menuKeys {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, mainType))
		}
		onResp(sb.String())
	} else {
		// 展示特定频道的子频道名
		mainType := c.tryConvertFromIndexToMainType(splited[1])
		if im, ok := c.menu[mainType]; ok {
			onResp(c.intelMenuStr(mainType, im))
		} else {
			onResp(fmt.Sprintf("channel `%s` 不存在", mainType))
		}
	}
}

func (c *Service) intelMenuStr(mainType string, im IntelMenu) string {
	if im.SubtypeUncertain {
		return fmt.Sprintf("%s:\n%s", mainType, im.SybTypeUncertainReason)
	} else if len(im.SubTypes) == 0 {
		return fmt.Sprintf("[%s]:\n没有子频道", mainType)
	} else {
		sb := strings.Builder{}
		sb.WriteString(fmt.Sprintf("[%s]的当前子频道为:\n", mainType))
		i := 0
		for k := range im.SubTypes {
			i++
			sb.WriteString(fmt.Sprintf("%d. %s\n", i, k))
		}
		return sb.String()
	}
}

func (c *Service) checkIntelType(mainType, subType string) (bool, string) {
	if im, ok := c.menu[mainType]; !ok {
		return false, fmt.Sprintf("频道[%s]不存在", mainType)
	} else {
		if len(subType) == 0 {
			return true, ""
		} else if im.SubtypeUncertain {
			return true, ""
		} else if _, ok := im.SubTypes[subType]; ok {
			return true, ""
		} else {
			return false, fmt.Sprintf("频道[%s]下不存在子频道[%s]", mainType, subType)
		}
	}
}

func (c *Service) tryConvertFromIndexToMainType(mainType string) string {
	if index, ok := util.String2Int(mainType); ok {
		index--
		if index >= 0 && index < len(c.menuKeys) {
			return c.menuKeys[index]
		}
	}
	return mainType
}

func (c *Service) onCmdMy(uid, nick string, onResp func(string)) {
	str := c.filter.userFilterStr(uid)
	onResp(str)
}

func (c *Service) onCmdSubscribe(splited []string, uid, nick string, onResp func(string)) {
	if len(splited) < 2 {
		onResp(fmt.Sprintf("not enough param for command `s`, type help for more info"))
		return
	}

	mainType := c.tryConvertFromIndexToMainType(splited[1])
	if ok, msg := c.checkIntelType(mainType, ""); !ok {
		onResp(msg)
		return
	}

	c.filter.subscribeType(uid, nick, mainType)
	onResp(fmt.Sprintf("subscribe [%s] done", mainType))
}

func (c *Service) onCmdUnsubscribe(splited []string, uid, nick string, onResp func(string)) {
	if len(splited) < 2 {
		onResp(fmt.Sprintf("not enough param for command `us`, type help for more info"))
		return
	}

	mainType := c.tryConvertFromIndexToMainType(splited[1])
	if ok, msg := c.checkIntelType(mainType, ""); !ok {
		onResp(msg)
		return
	}

	c.filter.unsubscribeType(uid, nick, mainType)
	onResp(fmt.Sprintf("unsubscribe [%s] done", mainType))
}

func (c *Service) onCmdSubscribeSub(splited []string, uid, nick string, onResp func(string)) {
	if len(splited) < 3 {
		onResp(fmt.Sprintf("not enough param for command `ss`, type help for more info"))
		return
	}

	mainType := c.tryConvertFromIndexToMainType(splited[1])
	subType := splited[2]
	if ok, msg := c.checkIntelType(mainType, subType); !ok {
		onResp(msg)
		return
	}

	c.filter.userAddSubtypeWhiteList(uid, mainType, subType)
	onResp(fmt.Sprintf("[%s] added to [%s]'s white list", subType, mainType))
}

func (c *Service) onCmdUnsubscribeSub(splited []string, uid, nick string, onResp func(string)) {
	if len(splited) < 3 {
		onResp(fmt.Sprintf("not enough param for command `uss`, type help for more info"))
		return
	}

	mainType := c.tryConvertFromIndexToMainType(splited[1])
	subType := splited[2]
	if ok, msg := c.checkIntelType(mainType, subType); !ok {
		onResp(msg)
		return
	}

	c.filter.userRemoveSubtypeWhiteList(uid, mainType, subType)
	onResp(fmt.Sprintf("[%s] removed from [%s]'s white list", subType, mainType))
}

func (c *Service) onCmdExcludeSub(splited []string, uid, nick string, onResp func(string)) {
	if len(splited) < 3 {
		onResp(fmt.Sprintf("not enough param for command `xs`, type help for more info"))
		return
	}

	mainType := c.tryConvertFromIndexToMainType(splited[1])
	subType := splited[2]
	if ok, msg := c.checkIntelType(mainType, subType); !ok {
		onResp(msg)
		return
	}

	c.filter.userAddSubtypeBlackList(uid, mainType, subType)
	onResp(fmt.Sprintf("[%s] added to [%s]'s black list", subType, mainType))
}

func (c *Service) onCmdUnexcludeSub(splited []string, uid, nick string, onResp func(string)) {
	if len(splited) < 3 {
		onResp(fmt.Sprintf("not enough param for command `uxs`, type help for more info"))
		return
	}

	mainType := c.tryConvertFromIndexToMainType(splited[1])
	subType := splited[2]
	if ok, msg := c.checkIntelType(mainType, subType); !ok {
		onResp(msg)
		return
	}

	c.filter.userRemoveSubtypeBlackList(uid, mainType, subType)
	onResp(fmt.Sprintf("[%s] removed from [%s]'s black list", subType, mainType))
}

func (c *Service) onCmdClearSubchannelSettings(splited []string, uid, nick string, onResp func(string)) {
	if len(splited) < 2 {
		onResp(fmt.Sprintf("not enough param for command `css`, type help for more info"))
		return
	}

	mainType := c.tryConvertFromIndexToMainType(splited[1])
	if ok, msg := c.checkIntelType(mainType, ""); !ok {
		onResp(msg)
		return
	}

	c.filter.userClearSubtypeWhiteList(uid, mainType)
	c.filter.userClearSubtypeBlackList(uid, mainType)
	onResp(fmt.Sprintf("[%s]'s white/black list cleared", mainType))
}
