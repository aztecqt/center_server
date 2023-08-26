/*
 * @Author: aztec
 * @Date: 2022-06-15 13:41

 * @Description:中央服务器的通讯协议定义
 *
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */

package stratergys

import (
	"encoding/json"

	"github.com/aztecqt/dagger/util/udpsocket"
)

const OpPingReq = "ping_req"
const OpPingResp = "ping_resp"
const OpQuitRpt = "quit"
const OpActiveStatusRpt = "as_rpt"
const OpCmdReq = "cmd_req"
const OpCmdResp = "cmd_resp"
const OpQuantEventReport = "qevent_rpt"
const OpQuantEventBroadcast = "qevent_bct"
const OpQuantEventResp = "qevent_resp"

// 策略->服务器
type PingReq struct {
	udpsocket.Header
	GUID  string `json:"guid"`
	Name  string `json:"name"`
	Class string `json:"class"`
}

func NewPingReq(guid, name, class string) []byte {
	req := PingReq{
		GUID:  guid,
		Name:  name,
		Class: class,
	}
	req.OP = OpPingReq
	b, _ := json.Marshal(&req)
	return b
}

// 服务器->策略
type PingResp struct {
	udpsocket.Header
	Result string `json:"rst"`
}

func NewPingResp(rst string) []byte {
	resp := PingResp{
		Result: rst,
	}
	resp.OP = OpPingResp
	b, _ := json.Marshal(&resp)
	return b
}

// 策略->服务器
type QuitRpt struct {
	udpsocket.Header
	GUID string `json:"guid"`
}

func NewQuitRpt(guid string) []byte {
	rpt := QuitRpt{}
	rpt.GUID = guid
	rpt.OP = OpQuitRpt
	b, _ := json.Marshal(&rpt)
	return b
}

// 用户指令，服务器->策略
type Command struct {
	udpsocket.Header
	Cmd     string `json:"cmd"`
	Webhook string `json:"wbh"`
}

func NewCommandReq(cmd, webhook string) []byte {
	req := Command{
		Cmd:     cmd,
		Webhook: webhook,
	}
	req.OP = OpCmdReq
	b, _ := json.Marshal(&req)
	return b
}

// 用户指令，策略->服务器
type CommandResp struct {
	udpsocket.Header
	Name    string `json:"name"`
	Result  string `json:"rst"`
	Webhook string `json:"wbh"`
}

func NewCommandResp(name, rst, webhook string) []byte {
	resp := CommandResp{
		Name:    name,
		Result:  rst,
		Webhook: webhook,
	}
	resp.OP = OpCmdResp
	b, _ := json.Marshal(&resp)
	return b
}

// 一个量化事件
type QuantEvent struct {
	EventName  string            `json:"ename"`
	EventParam map[string]string `json:"eparam"`
}

// 量化事件，由服务器广播给策略
type QuantEventBroadcast struct {
	udpsocket.Header
	QuantEvent
	EventSeq int `json:"eseq"` // 事件序列号，同一个序列号的事件只应处理一次。策略上报时填-1
}

func NewQuantEventBroadcast(seq int, ename string, eparam map[string]string) []byte {
	qe := QuantEventBroadcast{}
	qe.OP = OpQuantEventBroadcast
	qe.EventSeq = seq
	qe.EventName = ename
	qe.EventParam = eparam
	b, _ := json.Marshal(&qe)
	return b
}

// 量化事件，策略->服务器
type QuantEventResp struct {
	udpsocket.Header
	GUID     string `json:"guid"`
	Handled  bool   `json:"handled"`
	EventSeq int    `json:"eseq"` // 回复消息表示已经接收到了此事件
}

func NewQuantEventResp(seq int, guid string, handled bool) []byte {
	resp := QuantEventResp{}
	resp.EventSeq = seq
	resp.GUID = guid
	resp.Handled = handled
	resp.OP = OpQuantEventResp
	b, _ := json.Marshal(&resp)
	return b
}
