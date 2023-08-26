/*
 * @Author: aztec
 * @Date: 2023-07-01 15:42:51
 * @Description:  发送情报的客户端
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package csclient

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/aztecqt/center_server/server/intel"
	"github.com/aztecqt/dagger/util/logger"
	"github.com/aztecqt/dagger/util/network"
)

type IntelClient struct {
	url string
}

func NewIntelClient(url string) *IntelClient {
	s := new(IntelClient)
	s.url = url
	return s
}

func (s *IntelClient) SendIntelMenu(menu intel.IntelMenu) {
	b, _ := json.Marshal(menu)
	url := fmt.Sprintf("%s/intel/menu", s.url)
	network.HttpCall(url, "POST", string(b), nil, func(r *http.Response, err error) {
		if err != nil {
			logger.LogImportant("intel_client", err.Error())
		}
	})
}

func (s *IntelClient) SendIntel(intel intel.Intel) {
	b, _ := json.Marshal(intel)
	url := fmt.Sprintf("%s/intel/new", s.url)
	network.HttpCall(url, "POST", string(b), nil, func(r *http.Response, err error) {
		if err != nil {
			logger.LogImportant("intel_client", err.Error())
		}
	})
}
