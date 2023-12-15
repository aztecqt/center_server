/*
 * @Author: aztec
 * @Date: 2023-01-09 18:01:33
 * @Description: 活动状态提醒。活动状态注册后，如果不再继续活动，则会发送提醒。
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package activestatus

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/aztecqt/center_server/server/web"
	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/dingtalk"
	"github.com/aztecqt/dagger/util/logger"
)

const logPrefix = "service-active_status"

type activeStatus struct {
	code            int64
	activeTime      time.Time // 上次活动时间
	maxStuckTimeSec int       // 最大可卡死时间
	lastNoticeTime  time.Time // 上次提醒时间
	noticeCount     int       // 提醒次数
}

func (a *activeStatus) String() string {
	return fmt.Sprintf(
		"code:%d\tactiveAt:%dsec\tmaxStuck:%dsec\tlastNotice:%s\tnoticeCount:%d",
		a.code, int(time.Now().Sub(a.activeTime).Seconds()), a.maxStuckTimeSec, a.lastNoticeTime.Format(time.DateTime), a.noticeCount)
}

type Service struct {
	// 活动状态：GUID->statusName->activeStatus
	activeStatus   map[string]map[string]activeStatus
	muActiveStatus sync.Mutex
	ding           *dingtalk.Notifier
	dingAdminMob   int64
}

func (s *Service) Start(webservice *web.Service, ding *dingtalk.Notifier, dingAdminMob int64) {
	s.activeStatus = make(map[string]map[string]activeStatus)
	s.ding = ding
	s.dingAdminMob = dingAdminMob
	webservice.RegisterPath("/active_status/update", s.onHttpUpdate)
	webservice.RegisterPath("/active_status/quit", s.onHttpQuit)
	webservice.RegisterPath("/active_status/list", s.onHttpList)
	logger.LogImportant(logPrefix, "started")
	go s.update()
}

func (s *Service) update() {
	ticker := time.NewTicker(time.Second)

	for {
		<-ticker.C
		func() {
			s.muActiveStatus.Lock()
			defer s.muActiveStatus.Unlock()

			now := time.Now()
			for guid, statusMap := range s.activeStatus {
				for statusName, status := range statusMap {
					if status.noticeCount < 3 { // 最大通知3次
						if now.Unix()-status.activeTime.Unix() > int64(status.maxStuckTimeSec) { // 超过最大容忍时间
							if now.Unix()-status.lastNoticeTime.Unix() > 300 { // 5分钟提醒一次
								// 发送提醒
								noticeText := fmt.Sprintf("[%s]的状态[%s]已经累计卡死%d秒", guid, statusName, now.Unix()-status.activeTime.Unix())
								s.ding.SendTextByMob(noticeText, s.dingAdminMob)

								// 记录发送状态
								status.lastNoticeTime = now
								status.noticeCount++
							}
						}

						statusMap[statusName] = status
					} else {
						// 超过最大通知次数后删除
						delete(statusMap, statusName)
					}
				}
			}
		}()
	}
}

func (s *Service) refresh(guid, statusName string, code int64, maxStuckTimeSec int) {
	s.muActiveStatus.Lock()
	defer s.muActiveStatus.Unlock()

	if _, ok := s.activeStatus[guid]; !ok {
		s.activeStatus[guid] = make(map[string]activeStatus)
	}

	statusMap := s.activeStatus[guid]

	if _, ok := statusMap[statusName]; !ok {
		statusMap[statusName] = activeStatus{}
	}

	status := statusMap[statusName]

	if code > status.code {
		status.activeTime = time.Now()
		status.noticeCount = 0
		status.code = code
	}

	status.maxStuckTimeSec = maxStuckTimeSec
	statusMap[statusName] = status
}

func (s *Service) clear(guid string) {
	s.muActiveStatus.Lock()
	defer s.muActiveStatus.Unlock()
	delete(s.activeStatus, guid)
}

// /active_status/update?guid=xxxxx&name=xxx&code=12345&max_stuck_sec=600
// guid：进程的唯一标识
// name：状态名
// code：状态码。是一个不断增长的数值。一旦停止增长则认为卡住了
// max_stuck：最大卡死时长
func (s *Service) onHttpUpdate(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "GET" {
		q := r.URL.Query()
		guid := q.Get("guid")
		name := q.Get("name")
		scode := q.Get("code")
		sMaxStuck := q.Get("max_stuck")
		if len(guid) == 0 {
			io.WriteString(w, "missing guid")
			return
		}

		if len(name) == 0 {
			io.WriteString(w, "missing name")
			return
		}

		if len(scode) == 0 {
			io.WriteString(w, "missing code")
			return
		}

		if len(sMaxStuck) == 0 {
			io.WriteString(w, "missing max_stuck")
			return
		}

		code, ok := util.String2Int64(scode)
		if !ok {
			io.WriteString(w, "can't convert code to int")
			return
		}

		maxStuck, ok := util.String2Int(sMaxStuck)
		if !ok {
			io.WriteString(w, "can't convert max_stuck to int")
			return
		}

		s.refresh(guid, name, code, maxStuck)
		io.WriteString(w, "ok")
	}
}

// /active_status/quit?guid=xxxxx
func (s *Service) onHttpQuit(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "GET" {
		q := r.URL.Query()
		guid := q.Get("guid")
		if len(guid) == 0 {
			io.WriteString(w, "missing guid")
			return
		}

		s.clear(guid)
		io.WriteString(w, "ok")
	}
}

// /active_status/list
type asStatusItem struct {
	guid       string
	statusName string
	text       string
}

func (s *Service) onHttpList(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("active status entity:%d\n", len(s.activeStatus)))

	items := make([]asStatusItem, 0)
	for guid, status := range s.activeStatus {
		for s, as := range status {
			items = append(items, asStatusItem{
				guid:       guid,
				statusName: s,
				text:       as.String(),
			})
		}
	}

	slices.SortFunc(items, func(a, b asStatusItem) int {
		return strings.Compare(a.guid, b.guid)*10 + strings.Compare(a.statusName, b.statusName)
	})

	for i, asi := range items {
		sb.WriteString(fmt.Sprintf("%d. guid:%s\tstatus:%s\t%s\n", i+1, asi.guid, asi.statusName, asi.text))
	}

	io.WriteString(w, sb.String())
}
