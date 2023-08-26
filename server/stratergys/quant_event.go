/*
 * @Author: aztec
 * @Date: 2022-09-21
 * @Description:一条发往策略程序的量化事件
 *
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */

package stratergys

import (
	"net"
	"time"

	"github.com/aztecqt/dagger/util/udpsocket"
)

// 负责把一个quantEvent投递到策略端
type quantEvent2Stratergy struct {
	eData        []byte
	addr         *net.UDPAddr
	us           udpsocket.Socket
	guid         string
	seq          int
	acknowledged bool
	finished     bool
}

func newQuantEvent2Stratergy(
	seq int,
	ename string,
	eparam map[string]string,
	addr *net.UDPAddr,
	us udpsocket.Socket,
	guid string) *quantEvent2Stratergy {
	sender := new(quantEvent2Stratergy)
	sender.eData = NewQuantEventBroadcast(seq, ename, eparam)
	sender.addr = addr
	sender.us = us
	sender.guid = guid
	sender.seq = seq
	sender.acknowledged = false
	sender.finished = false
	return sender
}

func (s *quantEvent2Stratergy) run() {
	// 首次立即发送
	s.us.SendTo(s.eData, s.addr)

	// 未收到确认之前，一秒2次，重复10秒
	ticker := time.NewTicker(time.Millisecond * 500)
	sendCount := 0
	for {
		<-ticker.C
		if s.acknowledged {
			break
		}

		s.us.SendTo(s.eData, s.addr)
		sendCount++
		if sendCount > 20 {
			break
		}
	}
	s.finished = true
}
