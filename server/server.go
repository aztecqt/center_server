/*
 * @Author: aztec
 * @Date: 2022-06-15 13:41
 * @Description:
 * 服务器的具体功能都拆分成了各个service
 * 这里只要负责创建一些基础模块，然后启动各个service即可
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */

package server

import (
	"github.com/aztecqt/center_server/server/activestatus"
	"github.com/aztecqt/center_server/server/antntf"
	"github.com/aztecqt/center_server/server/file"
	"github.com/aztecqt/center_server/server/intel"
	"github.com/aztecqt/center_server/server/quantevent"
	"github.com/aztecqt/center_server/server/stratergys"
	"github.com/aztecqt/center_server/server/web"
	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/dingtalk"
)

const logPrefix = "server"

type LaunchConfig struct {
	ServerAddr   string                  `json:"server_addr"`
	RedisConfig  util.RedisConfig        `json:"redis_config"`
	DingConfig   dingtalk.NotifierConfig `json:"ding_config"`
	DingAdminMob int64                   `json:"ding_admin_mob"`

	Services struct {
		Web struct {
			Enabled bool `json:"enabled"`
			Port    int  `json:"port"`
		} `json:"web"`
		Stratergy struct {
			Enabled       bool   `json:"enabled"`
			Port          int    `json:"port"`
			DingbotSecret string `json:"ding_bot_secret"`
		} `json:"stratergy"`
		ActiveStatus struct {
			Enabled bool `json:"enabled"`
		} `json:"active_status"`
		Intel struct {
			Enabled       bool   `json:"enabled"`
			DingbotSecret string `json:"ding_bot_secret"`
		} `json:"intel"`
		QuantEvent struct {
			Enabled bool `json:"enabled"`
		} `json:"quant_event"`
		FileServer struct {
			Enabled bool `json:"enabled"`
			Port    int  `json:"port"`
		} `json:"file_server"`
		AntNotify struct {
			Enabled        bool   `json:"enabled"`
			RedisAddr      string `json:"redis_addr"`
			RedisPass      string `json:"redis_pass"`
			RedisDB        int    `json:"redis_db"`
			FileServerPort int    `json:"file_server_port"`
		} `json:"ant_ntf"`
	} `json:"services"`
}

type CenterServer struct {
	ding *dingtalk.Notifier
	rc   *util.RedisClient
}

func (s *CenterServer) Start(lc LaunchConfig) {
	if lc.DingConfig.AgentId > 0 {
		s.ding = new(dingtalk.Notifier)
		s.ding.Init(lc.DingConfig)
	}

	if len(lc.RedisConfig.Addr) > 0 {
		s.rc = new(util.RedisClient)
		s.rc.InitFromConfig(lc.RedisConfig)
	}

	service_web := new(web.Service)
	service_stratergys := new(stratergys.Service)
	service_activeStatus := new(activestatus.Service)
	service_intel := new(intel.Service)
	service_quantEvent := new(quantevent.Service)
	service_fileserver := new(file.Service)
	service_antdingntf := new(antntf.Service)

	if lc.Services.Web.Enabled {
		service_web.Start(lc.Services.Web.Port)
	}

	if lc.Services.Stratergy.Enabled {
		service_stratergys.Start(service_web, lc.Services.Stratergy.Port, lc.Services.Stratergy.DingbotSecret)
	}

	if lc.Services.ActiveStatus.Enabled {
		service_activeStatus.Start(service_web, s.ding, lc.DingAdminMob)
	}

	if lc.Services.Intel.Enabled {
		service_intel.Start(service_web, s.ding, s.rc, lc.DingAdminMob, lc.Services.Intel.DingbotSecret)
	}

	if lc.Services.QuantEvent.Enabled {
		service_quantEvent.Start(service_web)
	}

	if lc.Services.FileServer.Enabled {
		service_fileserver.Start(service_web, lc.Services.FileServer.Port)
	}

	if lc.Services.AntNotify.Enabled {
		service_antdingntf.Start(
			service_web,
			lc.Services.AntNotify.RedisAddr,
			lc.Services.AntNotify.RedisPass,
			lc.Services.AntNotify.RedisDB,
			lc.ServerAddr,
			lc.Services.AntNotify.FileServerPort)
	}
}
