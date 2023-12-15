/*
	@Author: aztec

/*
  - @Date: 2023-01-09 17:55:06
  - @Description:
  - 提供策略列表管理/钉钉与策略之间的指令交互等功能/量化事件广播等功能
    *
  - Copyright (c) 2023 by aztec, All Rights Reserved.
*/
package stratergys

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aztecqt/center_server/dingbot"
	"github.com/aztecqt/center_server/server/web"
	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/logger"
	"github.com/aztecqt/dagger/util/udpsocket"
)

const logPrefix = "service-stratergys"

var instance *Service

func Instance() *Service {
	return instance
}

type Service struct {
	// 用于跟策略程序通讯
	us udpsocket.Socket

	// 策略列表 guid-stratergy
	stratergys     map[string]*Stratergy
	stratergyNames []string
	stratergyGuids []string
	muStratergys   sync.Mutex

	// 当前选中的策略
	connectedStratergy *Stratergy

	// 发往策略的QuantEvent
	sendingQuantEvent   []*quantEvent2Stratergy
	muSendingQuantEvent sync.Mutex
	quantEventSeqAcc    int

	// 用于验证丁丁机器人的消息
	dingBotSecret string
}

func (s *Service) Start(webservice *web.Service, localPort int, dingBotSecret string) {
	s.stratergys = make(map[string]*Stratergy)
	s.stratergyNames = make([]string, 0)
	s.stratergyGuids = make([]string, 0)
	s.sendingQuantEvent = make([]*quantEvent2Stratergy, 0)
	s.dingBotSecret = dingBotSecret

	// 处理策略交互中心的消息
	webservice.RegisterPath("/dingbots/stratergy", s.onHttp_DingMsg)

	// 启动本地监听（连接策略程序）
	s.us = udpsocket.Socket{}
	if !s.us.Listen(localPort, s.onRecvUDPMsg) {
		logger.LogImportant(logPrefix, "listen at port %d failed", localPort)
	} else {
		logger.LogImportant(logPrefix, "listening started")
	}

	// 启动策略维护线程
	go s.update()

	// 启动命令行输入
	go func() {
		input := bufio.NewScanner(os.Stdin)
		for input.Scan() {
			line := input.Text()
			s.onCommand(line, func(resp string, _ bool) {
				fmt.Println(resp)
			})
		}
	}()

	instance = s
	logger.LogImportant(logPrefix, "started")
}

// 钉钉机器人“策略交互中心”收到的私聊消息
func (s *Service) onHttp_DingMsg(w http.ResponseWriter, r *http.Request) {
	// 解析出钉钉消息
	msg := dingbot.ParseDingMessage(w, r, s.dingBotSecret)
	if msg == nil {
		return
	}

	text := msg.Text.Content
	logger.LogInfo(logPrefix, "receive ding msg: %s", text)

	// 先尝试本地命令行解析
	s.onCommand(text, func(resp string, processed bool) {
		if processed {
			dingbot.ReplayTextMsg(resp, msg.Webhook) // 自己处理过
			if text == "help" && s.connectedStratergy != nil {
				// 发给策略处理
				s.sendCmdToCurrentStratergy(msg.Text.Content, msg.Webhook)
			}
		} else {
			if s.connectedStratergy == nil {
				dingbot.ReplayTextMsg(resp, msg.Webhook)
			} else {
				// 发给策略处理
				s.sendCmdToCurrentStratergy(msg.Text.Content, msg.Webhook)
			}
		}
	})
}

// 向策略发送一个量化事件
func (s *Service) SendQuantEventRaw(args []string) int {
	//格式：ename eparam val eparam val ...
	ename := args[0]
	eparam := make(map[string]string)
	for i := 1; i < len(args)-1; i = i + 2 {
		eparam[args[i]] = args[i+1]
	}
	return s.SendQuantEvent(ename, eparam)
}

// 向策略发送一个量化事件
func (s *Service) SendQuantEvent(ename string, eparam map[string]string) int {
	s.muStratergys.Lock()
	defer s.muStratergys.Unlock()

	// 目前是发给所有策略
	sended := 0
	for _, stg := range s.stratergys {
		qes := newQuantEvent2Stratergy(s.quantEventSeqAcc, ename, eparam, stg.addr, s.us, stg.guid)
		s.quantEventSeqAcc++
		go qes.run()
		s.muSendingQuantEvent.Lock()
		s.sendingQuantEvent = append(s.sendingQuantEvent, qes)
		s.muSendingQuantEvent.Unlock()
		logger.LogImportant(logPrefix, fmt.Sprintf("send quant-event(seq=%d, name=%s) to stratergy %s", qes.seq, ename, stg.guid))
		sended++
	}

	return sended
}

// 消息转发给策略服务器
func (s *Service) sendCmdToCurrentStratergy(cmd, webhook string) {
	if s.connectedStratergy != nil {
		req := NewCommandReq(cmd, webhook)
		addr := s.connectedStratergy.addr
		s.us.SendTo(req, addr)
		logger.LogInfo(logPrefix, "trans cmd `%s` to stratergy addr: %s", cmd, addr.String())
	}
}

// UDP消息
func (s *Service) onRecvUDPMsg(op string, data []byte, addr *net.UDPAddr) {
	switch op {
	case OpPingReq:
		// 策略发来的ping请求
		func() {
			s.muStratergys.Lock()
			defer s.muStratergys.Unlock()
			req := PingReq{}
			if err := json.Unmarshal(data, &req); err == nil {
				if stg, ok := s.stratergys[req.GUID]; ok {
					// 刷新aliveTime
					stg.aliveTime = time.Now()
				} else {
					// 创建新的策略镜像
					stg := new(Stratergy)
					stg.addr = addr
					stg.guid = req.GUID
					stg.name = req.Name
					stg.class = req.Class
					stg.aliveTime = time.Now()
					s.stratergys[stg.guid] = stg
					s.stratergyGuids = append(s.stratergyGuids, stg.guid)
					s.stratergyNames = append(s.stratergyNames, stg.name)
					logger.LogInfo(logPrefix, "stratergy [%s] is online", stg.name)
				}
			} else {
				logger.LogImportant(logPrefix, "unmarshal error: %s", string(data))
			}

			// 回消息
			resp := NewPingResp("ok")
			s.us.SendTo(resp, addr)
		}()
	case OpQuitRpt:
		// 策略通知服务器程序退出
		rpt := QuitRpt{}
		if err := json.Unmarshal(data, &rpt); err == nil {
			// 让策略下线
			s.stratergyOffline(rpt.GUID)
		}
	case OpCmdResp:
		// 策略发来的命令回复
		resp := CommandResp{}
		if err := json.Unmarshal(data, &resp); err == nil {
			dingbot.ReplayTextMsg(fmt.Sprintf("from [%s]:\n%s", resp.Name, resp.Result), resp.Webhook)
		} else {
			logger.LogImportant(logPrefix, "unmarshal error: %s", string(data))
		}
	case OpQuantEventResp:
		// 策略对QuantEvent做出响应
		resp := QuantEventResp{}
		if err := json.Unmarshal(data, &resp); err == nil {
			s.muSendingQuantEvent.Lock()
			for _, qes := range s.sendingQuantEvent {
				if qes.seq == resp.EventSeq && qes.guid == resp.GUID {
					qes.acknowledged = true
					logger.LogImportant(logPrefix, fmt.Sprintf("quant-event(seq=%d) responsed by stratergy %s, handled=%v", resp.EventSeq, resp.GUID, resp.Handled))
				}
			}
			s.muSendingQuantEvent.Unlock()
		} else {
			logger.LogImportant(logPrefix, "unmarshal error: %s", string(data))
		}
	}
}

func (s *Service) update() {
	ticker := time.NewTicker(time.Second)
	for {
		<-ticker.C

		// 清除不活动的策略
		func() {
			defer util.DefaultRecover()

			s.muStratergys.Lock()
			keys := make([]string, 0, len(s.stratergys))
			for k := range s.stratergys {
				keys = append(keys, k)
			}
			s.muStratergys.Unlock()

			for _, guid := range keys {
				if time.Since(s.stratergys[guid].aliveTime).Seconds() > 10 {
					// 10秒不活动的策略就清除
					s.stratergyOffline(guid)
				}
			}
		}()

		// 清除已经完毕的QuantEventSender
		func() {
			defer util.DefaultRecover()

			s.muSendingQuantEvent.Lock()
			for i, qes := range s.sendingQuantEvent {
				if qes.finished {
					util.SliceRemoveAt(s.sendingQuantEvent, i)
				}
			}
			defer s.muSendingQuantEvent.Unlock()
		}()
	}
}

func (s *Service) stratergyOffline(guid string) {
	s.muStratergys.Lock()
	defer s.muStratergys.Unlock()

	name := s.stratergys[guid].name
	logger.LogInfo(logPrefix, "stratergy [%s] is offline", name)

	for i, v := range s.stratergyGuids {
		if v == guid {
			s.stratergyGuids = util.SliceRemoveAt(s.stratergyGuids, i)
			logger.LogInfo(logPrefix, "stratergy [%s] is removed from guid list", guid)
		}
	}

	for i, v := range s.stratergyNames {
		if v == name {
			s.stratergyNames = util.SliceRemoveAt(s.stratergyNames, i)
			logger.LogInfo(logPrefix, "stratergy [%s] is removed from name list", name)
		}
	}

	delete(s.stratergys, guid)

	if s.connectedStratergy != nil && s.connectedStratergy.guid == guid {
		s.connectedStratergy = nil
	}
}

// as terminal
func (s *Service) onCommand(cmdLine string, onResp func(string, bool)) {
	splited := strings.Split(cmdLine, " ")

	cmd := splited[0]
	switch cmd {
	case "help":
		sb := strings.Builder{}
		sb.WriteString("1. ls\nlist all active stratergys\n")
		sb.WriteString("2. sklog n\ntoggle socket log\n")
		sb.WriteString("3. conn n\nconnect to stratergy by index\n")
		sb.WriteString("4. disc n\ndisconnect from current stratergy\n")
		sb.WriteString("5. qevent ename k1 v1 k2 v2...\ncreate a quant-event manually\n")
		onResp(sb.String(), true)
	case "ls": // list stratergy
		sb := strings.Builder{}
		sb.WriteString(fmt.Sprintf("alive stratergys count: %d\n", len(s.stratergyNames)))

		s.muStratergys.Lock()
		for i, v := range s.stratergyNames {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i, v))
		}
		s.muStratergys.Unlock()

		onResp(sb.String(), true)
	case "sklog":
		udpsocket.LogSocketDetail = !udpsocket.LogSocketDetail
		onResp(fmt.Sprintf("socket log switched to %v", udpsocket.LogSocketDetail), true)
	case "conn":
		if len(splited) < 2 {
			// 输出当前连接的策略
			if s.connectedStratergy == nil {
				onResp("no stratergy connected", true)
			} else {
				onResp(fmt.Sprintf("connected:[%s]\nclass: [%s]", s.connectedStratergy.name, s.connectedStratergy.class), true)
			}
		} else {
			// 根据索引连接某个策略
			func() {
				s.muStratergys.Lock()
				defer s.muStratergys.Unlock()
				if index, ok := util.String2Int(splited[1]); ok {
					if index >= 0 && index < len(s.stratergyGuids) {
						s.connectedStratergy = s.stratergys[s.stratergyGuids[index]]
						onResp(fmt.Sprintf("stratergy [%s] connected", s.connectedStratergy.name), true)
					} else {
						onResp("index out of range", true)
					}
				} else {
					onResp(fmt.Sprintf("invalid index: %s", splited[1]), true)
				}
			}()
		}
	case "disc":
		// 断开连接
		if s.connectedStratergy == nil {
			onResp("no stratergy connected", true)
		} else {
			onResp(fmt.Sprintf("disconnected from stratergy [%s]", s.connectedStratergy.name), true)
			s.connectedStratergy = nil
		}
	case "qevent":
		// 手动模拟QuantEvent
		// qevent ename k1 v1 k2 v2...
		if len(splited) < 2 {
			onResp("not enough param for command qevent", true)
			return
		}

		sended := s.SendQuantEventRaw(splited[1:])
		ename := splited[1]
		onResp(fmt.Sprintf("send event(%s) to %d stratergys", ename, sended), true)
	default:
		onResp("unknown command", false)
	}
}
