/*
 * @Author: aztec
 * @Date: 2023-07-01 15:34:39
 * @Description: 供策略使用的udp客户端
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package csclient

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/aztecqt/center_server/server/stratergys"

	"github.com/aztecqt/dagger/stratergy"
	"github.com/aztecqt/dagger/util/logger"
	"github.com/aztecqt/dagger/util/terminal"
	"github.com/aztecqt/dagger/util/udpsocket"
)

type StratergyClient struct {
	guid           string
	s              stratergy.Stratergy
	us             udpsocket.Socket
	latestRespTime time.Time
	logPrefix      string
	terminal       terminal.Terminal
	running        bool
}

func (sc *StratergyClient) Start(serverAddr string, serverPort int, guid string, s stratergy.Stratergy, tm terminal.Terminal, onQuit func()) {
	sc.guid = guid
	sc.s = s
	sc.terminal = tm
	sc.logPrefix = "csclient"

	go sc.run(serverAddr, serverPort, onQuit)
}

func (sc *StratergyClient) run(serverAddr string, serverPort int, onQuit func()) {
	sc.running = true

	// 监控程序正常退出
	go func() {
		osc := make(chan os.Signal, 1)
		signal.Notify(osc, syscall.SIGTERM, syscall.SIGINT)
		<-osc
		sc.reportQuit()
		if onQuit != nil {
			onQuit()
		}
		logger.LogImportant(sc.logPrefix, "program is quiting")
		sc.running = false
	}()

	// socket连接
	for {
		if sc.us.Connect(serverAddr, serverPort, sc.onRecv) {
			break
		} else {
			time.Sleep(time.Millisecond * 100)
		}
	}

	logger.LogImportant(sc.logPrefix, "connected to center server %s:%d", serverAddr, serverPort)

	// 保持心跳
	ticker := time.NewTicker(time.Second * 3)
	for sc.running {
		<-ticker.C
		req := stratergys.NewPingReq(sc.guid, sc.s.Name(), sc.s.Class())
		sc.us.Send(req)
	}
}

// 汇报退出
func (sc *StratergyClient) reportQuit() {
	logger.LogInfo(sc.logPrefix, "reporting quit")
	rpt := stratergys.NewQuitRpt(sc.guid)
	sc.us.Send(rpt)
}

func (sc *StratergyClient) onRecv(op string, data []byte, addr *net.UDPAddr) {
	switch op {
	case stratergys.OpPingResp:
		// 心跳返回
		resp := stratergys.PingResp{}
		if err := json.Unmarshal(data, &resp); err == nil {
			if resp.Result == "ok" {
				sc.latestRespTime = time.Now()
			}
		} else {
			logger.LogImportant(sc.logPrefix, "unmarshal PingResp failed, str=%s", string(data))
		}
	case stratergys.OpCmdReq:
		// 命令行输入
		req := stratergys.Command{}
		if err := json.Unmarshal(data, &req); err == nil {

			cmds := strings.Split(strings.ReplaceAll(req.Cmd, "\r\n", "\n"), "\n")

			for i, cmd := range cmds {
				sc.terminal.OnCommand(cmd, func(result string) {
					result = strings.Replace(result, "\"", "`", -1)
					fmt.Println(result)

					// 多条指令时，只有最后一条指令的结果，才反馈给CenterServer
					if i == len(cmds)-1 {
						resp := stratergys.NewCommandResp(sc.s.Name(), result, req.Webhook)
						sc.us.Send(resp)
					}
				})
			}
		} else {
			logger.LogImportant(sc.logPrefix, "unmarshal Command failed, str=%s", string(data))
		}
	case stratergys.OpQuantEventBroadcast:
		// 量化事件
		evt := stratergys.QuantEventBroadcast{}
		if err := json.Unmarshal(data, &evt); err == nil {
			// 量化事件转交给策略处理
			handled := sc.s.OnQuantEvent(evt.EventName, evt.EventParam)
			logger.LogImportant(sc.logPrefix, "receive QuantEvent, ename=%s, param=%s", evt.EventName, evt.EventParam)

			// 回复消息
			resp := stratergys.NewQuantEventResp(evt.EventSeq, sc.guid, handled)
			sc.us.Send(resp)
		} else {
			resp := stratergys.NewQuantEventResp(evt.EventSeq, sc.guid, false)
			sc.us.Send(resp)
			logger.LogImportant(sc.logPrefix, "unmarshal QuantEvent failed, str=%s", string(data))
		}
	default:
		logger.LogImportant(sc.logPrefix, "unknown op: %s", op)
	}
}
