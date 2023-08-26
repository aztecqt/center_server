/*
 * @Author: aztec
 * @Date: 2022-06-14 17:38
 * @LastEditors: aztec
 * @FilePath: \center_server\main.go
 * @Description:程序入口
 *
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */

package main

import (
	"center_server/server"
	"fmt"
	"os"
	"time"

	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/logger"
)

func main() {
	logger.Init(logger.SplitMode_ByHours, 72)
	logger.ConsleLogLevel = logger.LogLevel_Info

	// 读取profile名
	bpf, err := os.ReadFile("profile.txt")
	if err != nil {
		logger.LogImportant("main", "read profile.txt failed")
		time.Sleep(time.Second * 3)
		return
	} else {
		profileName := string(bpf)
		profileDir := fmt.Sprintf("profiles/%s", profileName)

		lc := server.LaunchConfig{}
		util.ObjectFromFile(fmt.Sprintf("%s/config.json", profileDir), &lc)
		svr := server.CenterServer{}
		svr.Start(lc)

		for {
			time.Sleep(time.Second)
		}
	}
}
