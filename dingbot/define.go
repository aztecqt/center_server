/*
 * @Author: aztec
 * @Date: 2022-06-15 10:28
 * @LastEditors: aztec
 * @FilePath: \center_server\dingbot\define.go
 * @Description:协议定义
 *
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */

package dingbot

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/logger"
	"github.com/aztecqt/dagger/util/network"
)

const (
	ConversationType_1v1   = 1
	ConversationType_Group = 2
)

const logPrefix = "dingbot"

// 客户端发送来的消息
type DingUserMsg struct {
	SenderNick       string `json:"senderNick"`
	SenderUserId     string `json:"senderStaffId"`
	ConversationType string `json:"conversationType"`
	Webhook          string `json:"sessionWebhook"`
	Text             struct {
		Content string `json:"content"`
	} `json:"text"`
}

func ParseDingMessage(w http.ResponseWriter, r *http.Request, secret string) *DingUserMsg {
	defer util.DefaultRecover()
	if r.Method == "POST" {
		// 验证签名
		ts := r.Header["Timestamp"][0]
		clientSign := r.Header["Sign"][0]
		calculatedSign, err := sign(fmt.Sprintf("%s\n%s", ts, secret), secret)
		if err == nil {
			if calculatedSign != clientSign {
				logger.LogImportant(logPrefix, "sign failed!")
				io.WriteString(w, "sign failed!")
				return nil
			}
		} else {
			logger.LogImportant(logPrefix, "generate sign str failed, err=%s", err.Error())
			io.WriteString(w, "internal error")
			return nil
		}
	} else {
		logger.LogImportant(logPrefix, "wrong method")
		io.WriteString(w, "wrong method")
		return nil
	}

	// 验证通过，解析消息
	msg := DingUserMsg{}
	body := make([]byte, 1024*128)
	n, err := r.Body.Read(body)
	if err == nil || err == io.EOF {
		err = json.Unmarshal(body[:n], &msg)
		if err != nil {
			logger.LogImportant(logPrefix, "parse body error, err=%s", err.Error())
			io.WriteString(w, "internal error")
			return nil
		}

		// 解析成功，返回
		return &msg
	} else {
		logger.LogImportant(logPrefix, "read body error, err=%s", err.Error())
		io.WriteString(w, "internal error")
		return nil
	}
}

func sign(content string, secretKey string) (string, error) {
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, err := mac.Write([]byte(content))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

func ReplayTextMsg(content, webhook string) {
	toWebhookTextMsg(webhook, fmt.Sprintf("%s\n%s", time.Now().Format("2006-01-02 15:04:05"), content))
}

// 顺着webhook返回一条消息
func toWebhookTextMsg(webhookUrl string, content string) {
	poststr := fmt.Sprintf(`{"text":{"content":"%s"},"msgtype":"text"}`, content)
	network.HttpCall(webhookUrl, "POST", poststr, network.JsonHeaders(), func(r *http.Response, err error) {
		logger.LogImportant(logPrefix, err.Error())
	})
}
