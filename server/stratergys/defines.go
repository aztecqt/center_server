/*
 * @Author: aztec
 * @Date: 2023-01-10 09:52:40
 * @Description:
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package stratergys

import (
	"net"
	"time"
)

// 策略程序在服务器的镜像
type Stratergy struct {
	guid      string
	name      string
	class     string
	addr      *net.UDPAddr
	aliveTime time.Time
}
