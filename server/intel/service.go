/*
 * @Author: aztec
 * @Date: 2023-01-09 11:46:02
 * @Description:
 * 情报分发服务
 * 从intel collector中剥离出来
 * 主要特性有：
 * 接受机器人"消息助手"发来的命令
 * 管理订阅者-情报频道的关系
 * 以http接口的方式提供情报入口（由intel_collector的各个子collector直接使用）
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package intel

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/aztecqt/center_server/dingbot"
	"github.com/aztecqt/center_server/server/web"

	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/dingtalk"
	"github.com/aztecqt/dagger/util/logger"
)

const logPrefix = "service-intel"
const picIdGlobal = "@lALPDeREZUXvbJDNAgDNAgA"

const (
	IntelRedisKey_Status      = "intel_status"
	IntelRedisField_LatestSeq = "latest_seq"
	IntelRedisKey_List        = "intel_list"
)

type Service struct {
	// 钉钉用户的订阅和过滤
	filter   *dingFilter
	menu     map[string]IntelMenu // 所有可供订阅的情报类型\子类型
	menuKeys []string

	// 用于接受机器人消息时的验证
	dingBotSecret string

	// 直接发送给钉钉客户端
	ding         *dingtalk.Notifier
	dingAdminMob int64

	// redis服务器用于暂存接收到的intel，供IntelSpeaker客户端使用
	rc           *util.RedisClient
	lastIntelSeq int
}

func (s *Service) Start(webservice *web.Service, ding *dingtalk.Notifier, rc *util.RedisClient, dingAdminMob int64, dingBotSecret string) {
	s.filter = new(dingFilter)
	s.filter.init()

	s.menu = make(map[string]IntelMenu)

	s.ding = ding
	s.dingAdminMob = dingAdminMob
	s.dingBotSecret = dingBotSecret

	// 创建redis连接
	s.rc = rc
	if idstr, ok := s.rc.HGet(IntelRedisKey_Status, IntelRedisField_LatestSeq); ok {
		s.lastIntelSeq = util.String2IntPanic(idstr)
	}

	webservice.RegisterPath("/intel/new", s.onNewIntel)
	webservice.RegisterPath("/intel/menu", s.onNewIntelMenu)
	webservice.RegisterPath("/dingbots/message_assist", s.onDingMessage_MessageAssist)
	logger.LogImportant(logPrefix, "started")
}

func (s *Service) onDingMessage_MessageAssist(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()

	// 尝试解析出钉钉消息
	msg := dingbot.ParseDingMessage(w, r, s.dingBotSecret)
	if msg == nil {
		return
	}

	// 解析成功
	logger.LogInfo(
		logPrefix,
		"recv msg from %s(%s), content=%s, webhook=%s",
		msg.SenderNick,
		msg.SenderUserId,
		msg.Text.Content,
		msg.Webhook)

	// 仅支持单聊
	if msg.ConversationType == "1" {
		cmd := fmt.Sprintf("%s %s %s", msg.Text.Content, msg.SenderUserId, msg.SenderNick)
		s.OnCommand(cmd, func(s string) {
			dingbot.ReplayTextMsg(s, msg.Webhook)
		})
	} else {
		dingbot.ReplayTextMsg("请私聊", msg.Webhook)
	}

	io.WriteString(w, "acknowledged")
}

func (s *Service) onNewIntelMenu(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "POST" {
		// TODO:加密验证?
		menu := IntelMenu{}
		body := make([]byte, 1024*128)
		n, err := r.Body.Read(body)
		if err == nil || err == io.EOF {
			err = json.Unmarshal(body[:n], &menu)
			if err != nil {
				logger.LogImportant(logPrefix, "parse body error, err=%s", err.Error())
				io.WriteString(w, "internal error")
			}

			// 解析成功，记录这个menu
			io.WriteString(w, "ok")
			s.menu[menu.Type] = menu
			keys := make([]string, 0, len(s.menu))
			for k := range s.menu {
				keys = append(keys, k)
			}
			s.menuKeys = keys
		} else {
			logger.LogImportant(logPrefix, "read body error, err=%s", err.Error())
			io.WriteString(w, "internal error")
		}
	}
}

func (s *Service) onNewIntel(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "POST" {
		// TODO:加密验证?
		intel := Intel{}
		body := make([]byte, 1024*128)
		n, err := r.Body.Read(body)
		if err == nil || err == io.EOF {
			err = json.Unmarshal(body[:n], &intel)
			if err != nil {
				logger.LogImportant(logPrefix, "parse body error, err=%s", err.Error())
				io.WriteString(w, "internal error")
			}

			// 解析成功，处理这个intel
			io.WriteString(w, "ok")
			s.processIntel(intel)
		} else {
			logger.LogImportant(logPrefix, "read body error, err=%s", err.Error())
			io.WriteString(w, "internal error")
		}
	}
}

func (s *Service) processIntel(intel Intel) {
	if intel.Level == 0 {
		intel.Content = fmt.Sprintf("%s\n[debug]", intel.Content)
	}

	// 分配流水号
	s.lastIntelSeq++
	intel.Seq = s.lastIntelSeq

	b, _ := json.Marshal(intel)
	logger.LogInfo(logPrefix, "processing intel: %s", string(b))

	// 发给钉钉
	if len(intel.DingType) > 0 {
		if intel.Level == 0 {
			// 只发送给管理员
			if intel.DingType == DingType_Link {
				s.ding.SendLinkByMob(intel.Url, picIdGlobal, intel.Title, intel.Content, s.dingAdminMob)
			} else if intel.DingType == DingType_Text {
				s.ding.SendTextByMob(intel.Title+"\n"+intel.Content, s.dingAdminMob)
			}
		} else {
			// 发送给订阅者
			uids := s.filter.findMatchedUsers(intel.Type, intel.SubType)
			if intel.DingType == DingType_Link {
				s.ding.SendLinkByUid(intel.Url, picIdGlobal, intel.Title, intel.Content, uids...)
			} else if intel.DingType == DingType_Text {
				s.ding.SendTextByUid(intel.Title+"\n"+intel.Content, uids...)
			}
		}
		logger.LogInfo(logPrefix, "send to dingding done")
	}

	// 保存到redis
	if _, ok := s.rc.RPush(IntelRedisKey_List, string(b)); ok {
		s.rc.HSet(IntelRedisKey_Status, IntelRedisField_LatestSeq, intel.Seq) // 写入最新序列号
		s.rc.LTrim(IntelRedisKey_List, -50000, -1)                            // 保留一定数量的消息
	}
	logger.LogInfo(logPrefix, "save to redis done")
}
