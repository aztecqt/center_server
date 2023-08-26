/*
 * @Author: aztec
 * @Date: 2023-01-09 20:47:25
 * @Description: active status的客户端
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package activestatus

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/aztecqt/dagger/util/logger"
	"github.com/aztecqt/dagger/util/network"
)

type Sender struct {
	url string
}

func NewSender(url string) *Sender {
	s := new(Sender)
	if strings.Index(url, "http") != 0 {
		url = "http://" + url
	}
	s.url = url
	return s
}

func (s *Sender) Update(guid, name string, code int64, maxStuck int) {
	params := url.Values{}
	params.Set("guid", guid)
	params.Set("name", name)
	params.Set("code", fmt.Sprintf("%d", code))
	params.Set("max_stuck", fmt.Sprintf("%d", maxStuck))
	url := fmt.Sprintf("%s/active_status/update?%s", s.url, params.Encode())
	network.HttpCall(url, "GET", "", nil, func(r *http.Response, err error) {
		if err != nil {
			logger.LogImportant(logPrefix, err.Error())
		}
	})
}

func (s *Sender) Quit(guid string) {
	params := url.Values{}
	params.Set("guid", guid)
	url := fmt.Sprintf("%s/active_status/quit?%s", s.url, params.Encode())
	network.HttpCall(url, "GET", "", nil, func(r *http.Response, err error) {
		if err != nil {
			logger.LogImportant(logPrefix, err.Error())
		}
	})
}
