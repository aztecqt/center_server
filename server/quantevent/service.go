/*
 * @Author: aztec
 * @Date: 2023-01-11 10:08:28
 * @Description: 量化事件（发送给策略）
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package quantevent

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/aztecqt/center_server/server/stratergys"
	"github.com/aztecqt/center_server/server/web"

	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/logger"
)

const logPrefix = "service-quantevent"

type Service struct {
}

func (s *Service) Start(webservice *web.Service) {
	webservice.RegisterPath("/quantevent/new", s.onHttp_NewQuantEvent)
}

func (s *Service) onHttp_NewQuantEvent(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "POST" {
		qe := stratergys.QuantEvent{}
		body := make([]byte, 1024*128)
		n, err := r.Body.Read(body)
		if err == nil || err == io.EOF {
			err = json.Unmarshal(body[:n], &qe)
			if err != nil {
				logger.LogImportant(logPrefix, "parse body error, err=%s", err.Error())
				io.WriteString(w, "internal error")
			}

			// 解析成功，处理这个intel
			io.WriteString(w, "ok")
			stratergys.Instance().SendQuantEvent(qe.EventName, qe.EventParam)
		} else {
			logger.LogImportant(logPrefix, "read body error, err=%s", err.Error())
			io.WriteString(w, "internal error")
		}
	}
}
