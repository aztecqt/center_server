/*
 * @Author: aztec
 * @Date: 2023-01-11 10:22:17
 * @Description:
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package quantevent

import (
	"center_server/server/stratergys"
	"fmt"
	"net/http"

	"github.com/aztecqt/dagger/util/logger"
	"github.com/aztecqt/dagger/util/network"
)

// 向服务器提交一个量化事件
type Sender struct {
	url string
}

func NewSender(url string) *Sender {
	s := new(Sender)
	s.url = url
	return s
}

func (s *Sender) Send(event stratergys.QuantEvent) {
	url := fmt.Sprintf("%s/quantevent/new", s.url)
	network.HttpCall(url, "POST", "", nil, func(r *http.Response, err error) {
		if err != nil {
			logger.LogImportant(logPrefix, err.Error())
		}
	})
}
