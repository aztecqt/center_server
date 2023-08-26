/*
 * @Author: aztec
 * @Date: 2023-08-17 17:20:21
 * @Description: ant版的钉钉通知
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package antntf

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/aztecqt/center_server/server/web"
	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/dingtalk"
	"github.com/aztecqt/dagger/util/logger"
	"github.com/aztecqt/dagger/util/mathtools"
	"github.com/emirpasic/gods/sets/hashset"
)

const FreqCalculateDuration = time.Minute //计算发送频率的时间间隔（提示用）
const FreqMax = 10                        // 最大允许的发送频率。超过这个频率会给提示
const IntervalError = time.Minute * 3     // Error消息最小发送间隔
const IntervalFatal = time.Minute * 1     // Fatal消息最小发送间隔
const IntervalAlive = time.Minute         // sender保持Alive的最大间隔
const IntervalKeep = time.Hour            // 非Alive时间过长后，会直接删除sender
const ContentMaxLength = 512              // 消息最大字符数
const NotifyRecordRootPath = "./notify_record"

// 一个发送者的状态
type sender struct {
	name    string
	specMob string

	freqCal               mathtools.FreqCalculatorTimeWindow // 发送频率计算器
	freqTooHigh           bool                               // 频率过高标记。导致超频的最后一条消息，会附加一句提示信息。
	totalNftCount         int                                // 总计发送notify的次数
	latestBlockedNftCount int                                // 近期被拦截的消息数量。用于提示。
	lastErrorTime         time.Time                          // 上一次Error消息的发送时间
	lastFatalTime         time.Time                          // 上一次Fatal消息的发送时间
	lastAliveTime         time.Time                          // 上一次收到Alive的时间
	latestFatalNtf        *notify                            // 最近一条致命错误通知
	unreadNightMessage    int                                // 未读的夜间通知（normal级）
	started               bool
}

func (s *sender) string() string {
	sb := strings.Builder{}
	sb.WriteString(fmt.Sprintf("[sender:%s]%s\n", s.name, util.ValueIf(len(s.specMob) > 0, fmt.Sprintf("[spec mob:%s]", s.specMob), "")))
	sb.WriteString(fmt.Sprintf("活动中: %v\n", s.started))
	sb.WriteString(fmt.Sprintf("最近一次心跳：%d秒前\n", int(time.Now().Sub(s.lastAliveTime).Seconds())))
	freq := s.freqCal.Freq
	sb.WriteString(fmt.Sprintf("消息发送频率：%d/%d秒 (最大%d)\n", freq, int(FreqCalculateDuration.Seconds()), FreqMax))
	sb.WriteString(fmt.Sprintf("总消息数量:%d\n", s.totalNftCount))
	sb.WriteString(fmt.Sprintf("近期拦截数量:%d\n", s.latestBlockedNftCount))
	sb.WriteString(fmt.Sprintf("最近一次Error通知时间：%s\n", s.lastErrorTime.Format(time.DateTime)))
	sb.WriteString(fmt.Sprintf("最近一次Fatal通知时间：%s\n", s.lastFatalTime.Format(time.DateTime)))
	if s.latestFatalNtf == nil {
		sb.WriteString(fmt.Sprintf("当前Fatal通知：无 \n"))
	} else {
		sb.WriteString(fmt.Sprintf("当前Fatal通知：\n%s\n", s.latestFatalNtf.string(128)))
	}

	return sb.String()
}

// 服务
type Service struct {
	rc                *util.RedisClient
	dn                *dingtalk.Notifier
	fileServerRootDir string

	sysSender *sender
	senders   map[string]*sender // 对于每个idendify，保存一份状态数据
	senders2  []*sender
	muSenders sync.Mutex

	messages   map[int]*dingtalk.Message // 追踪所有message的发送状态
	muMessages sync.Mutex
}

func (s *Service) Start(
	webService *web.Service,
	redisAddr, redisPass string, redisDb int,
	serverAddr string, fileServicePort int) {
	//
	s.senders = make(map[string]*sender)
	s.senders2 = make([]*sender, 0)
	s.messages = make(map[int]*dingtalk.Message)

	// 连接redis
	s.rc = new(util.RedisClient)
	s.rc.InitFromConfig(util.RedisConfig{Addr: redisAddr, Password: redisPass, DB: redisDb})

	// 创建dingdingapi
	if dlc, err := s.getDingLaunchConfig(); err != nil {
		logger.LogPanic(logPrefix, "load ding launch config from redis failed")
	} else {
		s.dn = new(dingtalk.Notifier)
		s.dn.Init(dingtalk.NotifierConfig{Name: "ant-ding", AgentId: util.String2Int64Panic(dlc.AgentId), Key: dlc.Key, Secret: dlc.Secret})
	}

	// 创建系统Sender
	s.sysSender = s.createSender(specName_System, "")
	s.sysSender.lastAliveTime = time.Now()
	s.sysSender.started = true

	// 注册服务url
	webService.RegisterPath("/ant/notify/start", s.onStartOrKeepAlive)
	webService.RegisterPath("/ant/notify/alive", s.onStartOrKeepAlive)
	webService.RegisterPath("/ant/notify/stop", s.onReqStop)
	webService.RegisterPath("/ant/notify/send", s.onSendNotify)
	webService.RegisterPath("/ant/notify/status", s.onGetStatus)

	// 启动文件服务
	s.fileServerRootDir = fmt.Sprintf("%s:%d", serverAddr, fileServicePort)
	go func() {
		fileServer := http.Server{
			Addr:    fmt.Sprintf(":%d", fileServicePort),
			Handler: http.FileServer(http.Dir(NotifyRecordRootPath)),
		}
		fileServer.ListenAndServe()
	}()

	// 自循环
	go s.update()
	go s.updateMessageTracing()

	logger.LogInfo(logPrefix, "started")
}

// #region web处理
func (s *Service) onSendNotify(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "POST" {
		// 解析发送过来的ntf
		ntf := newNotify()
		body := make([]byte, 1024*16)
		n, err := r.Body.Read(body)
		if err == nil || err == io.EOF {
			err = json.Unmarshal(body[:n], &ntf)
			if err != nil {
				s.onInnerErrorWithNotify(logPrefix, "http请求解析错误, url=%s, err=%s", r.URL, err.Error())
				io.WriteString(w, "internal error")
			}

			// 解析成功，处理这个ntf
			msg := s.processNotify(ntf)
			io.WriteString(w, msg)
		} else {
			s.onInnerErrorWithNotify(logPrefix, "http请求解析错误, url=%s, err=%s", r.URL, err.Error())
			io.WriteString(w, "internal error")
		}
	}
}

func (s *Service) onReqStop(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "GET" {
		q := r.URL.Query()
		name := q.Get("name")
		if len(name) == 0 {
			io.WriteString(w, "missing name")
		}

		snd := s.findSender(name)
		if snd != nil {
			snd.started = false
			io.WriteString(w, "ok")
		} else {
			io.WriteString(w, "invalid sender name "+name)
		}
	}
}

func (s *Service) onStartOrKeepAlive(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "GET" {
		q := r.URL.Query()
		name := q.Get("name")
		specdMob := q.Get("specmob")
		if len(name) == 0 {
			io.WriteString(w, "missing name")
		}

		snd := s.findSender(name)
		if snd == nil {
			snd = s.createSender(name, specdMob)
		}

		snd.lastAliveTime = time.Now()
		snd.started = true
		io.WriteString(w, "ok")
	}
}
func (s *Service) onGetStatus(w http.ResponseWriter, r *http.Request) {
	defer util.DefaultRecover()
	if r.Method == "GET" {
		sb := strings.Builder{}
		sb.WriteString(fmt.Sprintf("======通知服务器======\n"))
		sb.WriteString(fmt.Sprintf("服务器本地时间：%s\n", time.Now().Format(time.DateTime)))
		sb.WriteString(fmt.Sprintf("Redis地址：%s\n", s.rc.Addr))
		sb.WriteString(fmt.Sprintf("历史记录查看：%s\n\n", s.fileServerRootDir))
		sb.WriteString(fmt.Sprintf("Sender数量：%d\n", len(s.senders)))
		for _, sender := range s.senders2 {
			sb.WriteString(sender.string())
			sb.WriteString("\n")
		}
		io.WriteString(w, sb.String())
	}
}

// #endregion

// #region 内部
// 1. 当一个sender的近期fatal错误存在，且发送延迟足够，则发送这个ntf
// 2. 检测sender的Alive状态，超时后通知
func (s *Service) update() {
	ticker := time.NewTicker(time.Second * 5)
	lastTime := time.Time{}
	for {
		<-ticker.C
		now := time.Now()

		// sender扫描
		func() {
			util.DefaultRecover()
			s.muSenders.Lock()
			keys := make([]string, 0, len(s.senders))
			for k := range s.senders {
				keys = append(keys, k)
			}
			s.muSenders.Unlock()

			for _, k := range keys {
				snd, ok := s.senders[k]
				if !ok {
					continue
				}

				// fatal ntf 重发检测
				if snd.latestFatalNtf != nil {
					ntf := snd.latestFatalNtf
					errLock := s.hasErrorLock(ntf.Name)
					if errLock {
						if now.Sub(snd.lastFatalTime) > IntervalFatal {
							// 未解锁，且时间间隔足够，执行重发
							snd.latestFatalNtf.Resend++
							s.sendNotify(snd.latestFatalNtf)
							snd.lastFatalTime = now
						}
					} else {
						// 已解锁，重置fatal状态
						snd.latestFatalNtf = nil
						snd.lastFatalTime = time.Time{}
					}
				}

				// alive检测
				if snd.started && now.Sub(snd.lastAliveTime) > IntervalAlive {
					snd.started = false
					ntf := newNotify()
					ntf.Level = NotifyLevel_Error
					ntf.LocalTimeStamp = now.UnixMilli()
					ntf.Name = snd.name
					ntf.Content = fmt.Sprintf("已失联超过%d秒", int64(IntervalAlive.Seconds()))
					s.sendNotify(ntf)
				}

				// 夜间未读消息通知
				if lastTime.Hour() == 8 && now.Hour() == 9 {
					// 9点了
					if snd.unreadNightMessage > 0 {
						ntf := newNotify()
						ntf.Level = NotifyLevel_Normal
						ntf.LocalTimeStamp = now.UnixMilli()
						ntf.Name = snd.name
						ntf.Content = fmt.Sprintf("昨夜有%d条未读消息（normal级别），可在网页端查看\n%s", snd.unreadNightMessage, s.fileServerRootDir)
						s.sendNotify(ntf)
						snd.unreadNightMessage = 0
					}
				}

				// 超时清理
				if now.Sub(snd.lastAliveTime) > IntervalKeep {
					logger.LogInfo(logPrefix, "sender [%s] is deleted for not alive", k)
					delete(s.senders, k)
					util.SliceRemove(s.senders2, snd)
				}
			}
		}()

		// 系统简报
		snd := s.findSender(specName_System)
		snd.lastAliveTime = time.Now()
		snd.started = true

		if lastTime.Hour() < 21 && now.Hour() >= 21 {
			ntf := newNotify()
			ntf.Level = NotifyLevel_Normal
			ntf.LocalTimeStamp = now.UnixMilli()
			ntf.Name = specName_System // 特殊名称
			sb := strings.Builder{}
			sb.WriteString("通知服务器运行正常\n")
			sb.WriteString(fmt.Sprintf("Sender数量：%d\n", len(s.senders2)))
			ntf.Content = sb.String()
			s.sendNotify(ntf)
		}

		lastTime = now
	}
}

func (s *Service) updateMessageTracing() {
	ticker := time.NewTicker(time.Second)
	for {
		<-ticker.C

		func() {
			util.DefaultRecover()
			// message发送结果追踪
			s.muMessages.Lock()
			keys := make([]int, 0, len(s.messages))
			for k := range s.messages {
				keys = append(keys, k)
			}
			s.muMessages.Unlock()

			for _, k := range keys {
				msg := s.messages[k]
				if msg.Status() == dingtalk.MessageStatus_Failed {
					delete(s.messages, k)
					logger.LogInfo(logPrefix, "notify send failed, id=%d, err count=%d", k, msg.ErrorCount())
				} else if msg.Status() == dingtalk.MessageStatus_Finished {
					delete(s.messages, k)
					logger.LogInfo(logPrefix, "notify send ok, id=%d, err count=%d", k, msg.ErrorCount())
				}
			}
		}()
	}
}

// 处理一条notify的发送。成功返回"ok"，失败返回原因
func (s *Service) processNotify(ntf *notify) string {
	logger.LogInfo(ntf.logPrefix(), "processing...")

	// 找出sender
	snd := s.findSender(ntf.Name)
	if snd == nil {
		s.onInnerErrorWithNotify(ntf.logPrefix(), "processNotify sender不存在")
		return fmt.Sprintf("no such sender:%s", ntf.Name)
	}

	// 解析出接收者的手机号
	mobs := s.parseReceiverMobs(ntf)
	if len(mobs) == 0 {
		s.onInnerErrorWithNotify(ntf.logPrefix(), "processNotify 无法定位接收人")
		return fmt.Sprintf("no receiver mobs for sender:%s", ntf.Name)
	}

	// 先把notify保存到本地文件中
	s.saveNotifyToFiles(ntf, mobs)

	// 计算此刻的发送频率
	snd.totalNftCount++
	snd.freqCal.Feed(FreqCalculateDuration)
	blockByFreq := false
	if snd.freqCal.Freq >= FreqMax {
		if !snd.freqTooHigh {
			// 第一条导致超频的消息，不block，仅提示
			ntf.ExtraLines = append(ntf.ExtraLines, fmt.Sprintf("[超频提醒] 近期通知发送频率已达%d次/分钟，请注意控频", snd.freqCal.Freq))
		} else {
			// 从第二次开始就要block了
			blockByFreq = true
		}

		snd.freqTooHigh = true
	} else {
		snd.freqTooHigh = false
	}

	// 频率验证统一进行
	if blockByFreq {
		snd.latestBlockedNftCount++
		msg := fmt.Sprintf("freq too high: %d", snd.freqCal.Freq)
		logger.LogInfo(ntf.logPrefix(), msg)
		return msg
	}

	// 根据notify的不同级别，做不同处理
	msg := ""
	switch ntf.Level {
	case NotifyLevel_Normal:
		msg = s.processNormalNotify(ntf)
	case NotifyLevel_Error:
		msg = s.processErrorNotify(ntf)
	case NotifyLevel_Fatal:
		msg = s.processFatalNotify(ntf)
	default:
		msg = fmt.Sprintf("unknown notify level: %d", ntf.Level)
	}

	logger.LogInfo(ntf.logPrefix(), msg)
	return fmt.Sprintf("%s (freq=%d)", msg, snd.freqCal.Freq)
}

// 普通消息：如果不是夜间的话直接发
func (s *Service) processNormalNotify(ntf *notify) string {
	if time.Now().Hour() >= 0 && time.Now().Hour() < 9 {
		snd := s.findSender(ntf.Name)
		snd.unreadNightMessage++
		return "normal notify block by night"
	} else {
		return s.sendNotify(ntf)
	}
}

// 错误消息：
// 如果当前不存在ErrorLock，则直接发
// 否则，如果距离上次Error提示时间超过锁定时间，也可以发
// 发送后，设置ErrorLock并记录上次Error发送时间
func (s *Service) processErrorNotify(ntf *notify) string {
	snd := s.findSender(ntf.Name) // 此处无需再做nil判断
	canSend := false
	cantSendReason := ""
	now := time.Now()
	errLock := s.hasErrorLock(ntf.Name)
	if !errLock {
		s.setErrorLock(ntf.Name)
		canSend = true
	} else {
		if now.Sub(snd.lastErrorTime) > IntervalError {
			canSend = true
		}

		cantSendReason = "error-lock blocked"
	}

	if canSend {
		msg := s.sendNotify(ntf)
		snd.lastErrorTime = now
		return msg
	} else {
		snd.latestBlockedNftCount++
		return cantSendReason
	}
}

// 致命消息：
// 如果当前不存在ErrorLock，则直接发
// 否则，覆盖sender的“最近Fatal消息记录”
// 然后另有专门进程负责定时发送
func (s *Service) processFatalNotify(ntf *notify) string {
	snd := s.findSender(ntf.Name) // 此处无需再做nil判断
	canSend := false
	cantSendReason := ""
	now := time.Now()
	errLock := s.hasErrorLock(ntf.Name)
	if !errLock {
		s.setErrorLock(ntf.Name)
		canSend = true
	} else {
		if now.Sub(snd.lastFatalTime) > IntervalFatal {
			canSend = true
		}

		cantSendReason = "error-lock blocked"
	}

	if canSend {
		msg := s.sendNotify(ntf)
		snd.lastFatalTime = now
		snd.latestFatalNtf = ntf
		return msg
	} else {
		snd.latestBlockedNftCount++
		snd.latestFatalNtf = ntf
		return cantSendReason
	}
}

// 执行发送
func (s *Service) sendNotify(ntf *notify) string {
	logger.LogInfo(ntf.logPrefix(), "try sending, content=%s", ntf.string(0))

	// 如果近期有被block的消息，给出提示
	mobs := s.parseReceiverMobs(ntf)
	snd := s.findSender(ntf.Name)
	if snd == nil {
		s.onInnerErrorWithNotify(ntf.logPrefix(), "sendNotify sender不存在")
		return "sender not found"
	}

	if snd.latestBlockedNftCount > 0 {
		ntf.ExtraLines = append(ntf.ExtraLines, fmt.Sprintf("[未读消息] 有%d条未读消息，可前往网页端查看\n%s", snd.latestBlockedNftCount, s.fileServerRootDir))
		snd.latestBlockedNftCount = 0
	}

	mobsn := make([]int64, 0)
	for _, v := range mobs {
		if n, ok := util.String2Int64(v); ok {
			mobsn = append(mobsn, n)
		}
	}

	msg := s.dn.SendTextByMob(ntf.string(ContentMaxLength), mobsn...)
	logger.LogInfo(ntf.logPrefix(), "sended to mobs: %v", mobsn)

	// 记录notify对象，跟踪发送状态，生成日志
	if msg != nil {
		s.muMessages.Lock()
		s.messages[ntf.Id] = msg
		s.muMessages.Unlock()
		return "ok"
	} else {
		return "send dingding failed"
	}
}

// 分析ntf应该发给谁
func (s *Service) parseReceiverMobs(ntf *notify) []string {
	mobset := hashset.New()

	// 尝试从策略配置中寻找接收者
	if len(ntf.Name) > 0 {
		if sc, err := s.getStratergyConfig(ntf.Name); err == nil {
			if pcfgs, err := s.getDingPersonCfgs(sc.DingUsers); err == nil {
				for _, dpc := range pcfgs {
					mobset.Add(dpc.Mob)
				}
			}
		}
	}

	// 尝试读取指定接受人
	snd := s.findSender(ntf.Name)
	if snd != nil {
		if len(snd.specMob) == 13 && util.IsIntNumber(snd.specMob) {
			// 是手机号
			mobset.Add(snd.specMob)
		}
	}

	// 特殊名称特殊处理
	if ntf.Name == specName_System {
		adminMobs := s.getAdminMobs()
		for _, mob := range adminMobs {
			mobset.Add(mob)
		}
	}

	// 如果未能定位到接受者（策略配置不存在、策略中没有指定接收者），则把管理员加进来
	if mobset.Size() == 0 {
		adminMobs := s.getAdminMobs()
		for _, mob := range adminMobs {
			mobset.Add(mob)
		}
		ntf.ExtraLines = append(ntf.ExtraLines, fmt.Sprintf("[未能定位到消息接收人]"))
	}

	mobs := make([]string, 0)
	vals := mobset.Values()
	for _, v := range vals {
		mobs = append(mobs, v.(string))
	}
	return mobs
}

// 获取管理员的手机号
func (s *Service) getAdminMobs() []string {
	mobs := make([]string, 0)
	if adminCfgs, err := s.getDingPersonCfgs_Admin(); err == nil {
		for _, dpc := range adminCfgs {
			mobs = append(mobs, dpc.Mob)
		}
	} else {
		s.onInnerErrorWithNotify(logPrefix, "getAdminMobs 读取ding person config失败")
	}
	return mobs
}

// 把一条通知保存到某人的目录中
func (s *Service) saveNotifyToFiles(ntf *notify, mobs []string) {
	defer util.DefaultRecover()
	for _, mob := range mobs {
		path := s.getNotifySavePath(ntf, mob)
		util.MakeSureDirForFile(path)
		if f, e := os.OpenFile(path, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666); e == nil {
			f.WriteString(ntf.string(0) + "\n")
			f.Close()
		}
	}
}

func (s *Service) getNotifySavePath(ntf *notify, mob string) string {
	tm := time.UnixMilli(ntf.LocalTimeStamp)
	path := fmt.Sprintf("%s/%s/%s.txt", NotifyRecordRootPath, mob, tm.Format("2006-01-02"))
	return path
}

func (s *Service) createSender(name, specMob string) *sender {
	snd := new(sender)
	snd.name = name
	snd.specMob = specMob
	s.muSenders.Lock()
	s.senders[name] = snd
	s.senders2 = append(s.senders2, snd)
	s.muSenders.Unlock()
	logger.LogInfo(logPrefix, "created sender [%s]", name)
	return snd
}

func (s *Service) findSender(name string) *sender {
	if snd, ok := s.senders[name]; ok {
		return snd
	} else {
		return nil
	}
}

// 从redis中查找指定策略
func (s *Service) getStratergyConfig(name string) (stratergyConfig, error) {
	str, ok := s.rc.HGet(RedisKey_StratergyConfig, name)
	sc := stratergyConfig{}
	if !ok {
		return sc, errors.New("read stratergy config failed")
	} else {
		err := json.Unmarshal([]byte(str), &sc)
		if err == nil {
			sc.parse()
			return sc, nil
		} else {
			return sc, err
		}
	}
}

// 从redis中读取钉钉启动配置
func (s *Service) getDingLaunchConfig() (dingLaunchConfig, error) {
	str, ok := s.rc.HGet(RedisKey_DingdingConfig, "key_config")
	lc := dingLaunchConfig{}
	if !ok {
		return lc, errors.New("read ding launch config failed")
	} else {
		err := json.Unmarshal([]byte(str), &lc)
		return lc, err
	}
}

// 从redis中读取指定钉钉人员配置
func (s *Service) getDingPersonCfgs(personNames []string) ([]dingPersonConfig, error) {
	str, ok := s.rc.HGet(RedisKey_DingdingConfig, "user_list")
	pcs := []dingPersonConfig{}
	results := []dingPersonConfig{}
	if !ok {
		return results, errors.New("read ding person config failed")
	} else {
		err := json.Unmarshal([]byte(str), &pcs)
		if err == nil {
			for _, pc := range pcs {
				name := pc.Name
				valid := false
				for _, n := range personNames {
					if name == n {
						valid = true
					}
				}

				if valid {
					results = append(results, pc)
				}
			}

			return results, nil
		} else {
			return results, err
		}
	}
}

// 从redis中读取管理员的人员配置
func (s *Service) getDingPersonCfgs_Admin() ([]dingPersonConfig, error) {
	str, ok := s.rc.HGet(RedisKey_DingdingConfig, "user_list")
	pcs := []dingPersonConfig{}
	results := []dingPersonConfig{}
	if !ok {
		return results, errors.New("read ding person config failed")
	} else {
		err := json.Unmarshal([]byte(str), &pcs)
		if err == nil {
			for _, pc := range pcs {
				if pc.IsAdmin != "0" {
					results = append(results, pc)
				}
			}

			return results, nil
		} else {
			return results, err
		}
	}
}

// 从redis读取发送者的errorlock标记
func (s *Service) hasErrorLock(name string) bool {
	str, ok := s.rc.HGet(RedisKey_Error, name)
	if !ok {
		return false
	} else {
		if strings.ToLower(str) == "true" {
			return true
		} else {
			return false
		}
	}
}

// 向redis中写入errorlock标记
func (s *Service) setErrorLock(name string) {
	if !s.rc.HSet(RedisKey_Error, name, "true") {
		s.onInnerError(logPrefix, fmt.Sprintf("设置%s的Error状态失败", name))
	}
}

// 发生内部错误。此时除了记录日志，还需要通知管理员。
func (s *Service) onInnerError(prefix, format string, a ...interface{}) string {
	msg := fmt.Sprintf(format, a...)
	logger.LogImportant(prefix, msg)
	return msg
}

func (s *Service) onInnerErrorWithNotify(prefix, format string, a ...interface{}) {
	msg := s.onInnerError(prefix, format, a...)

	mobs := s.getAdminMobs()
	mobsn := make([]int64, 0)
	for _, v := range mobs {
		if n, ok := util.String2Int64(v); ok {
			mobsn = append(mobsn, n)
		}
	}
	s.dn.SendTextByMob(msg, mobsn...)
}

// #endregion
