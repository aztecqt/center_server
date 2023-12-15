/*
 * @Author: aztec
 * @Date: 2023-08-07 15:08:26
 * @Description: 接受文件上传、并提供文件服务供外部查看
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */
package file

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/aztecqt/center_server/server/web"
	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/logger"
	"github.com/emirpasic/gods/sets/hashset"
)

var rootFolder = "./file_service"

const logPrefix = "service-file"

var validExt = hashset.New("html", "txt", "log", "csv", "json", "png", "jpg")

type Service struct {
}

func (s *Service) Start(webservice *web.Service, port int) {
	// 上传用的是主web服务器
	webservice.RegisterPath("/files/upload", s.onUploadRequest)
	webservice.RegisterPath("/files/delete", s.onDeleteRequest)

	// 启动文件服务
	server := http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: http.FileServer(http.Dir(rootFolder)),
	}

	go func() {
		err := server.ListenAndServe()
		if err != nil {
			logger.LogPanic(logPrefix, "ListenAndServe appear err: %s", err.Error())
		}
	}()
}

// 处理文件删除
func (s *Service) onDeleteRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == "DELETE" {
		q := r.URL.Query()
		fileName := q.Get("file_name")
		mainFolder := q.Get("main_folder")
		subFolder := q.Get("sub_folder")

		if len(fileName) == 0 {
			io.WriteString(w, "missing file_name")
		}

		if len(mainFolder) == 0 {
			io.WriteString(w, "missing main_folder")
		}

		filePath := ""
		if len(subFolder) == 0 {
			filePath = fmt.Sprintf("%s/%s/%s", rootFolder, mainFolder, fileName)
		} else {
			filePath = fmt.Sprintf("%s/%s/%s/%s", rootFolder, mainFolder, subFolder, fileName)
		}

		if exist, _ := util.PathExists(filePath); exist {
			e := os.Remove(filePath)
			if e == nil {
				io.WriteString(w, "ok")
			} else {
				io.WriteString(w, "error:"+e.Error())
			}
		}
	}
}

// 处理文件上传
// post参数：
// file：文件类型
// main_folder：一层目录（就是程序主体的名称，如策略名，必填）
// sub_folder：二层目录（选填）
func (s *Service) onUploadRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		// 获取参数
		mainFolder := r.FormValue("main_folder")
		subFolder := r.FormValue("sub_folder")

		if len(mainFolder) == 0 {
			io.WriteString(w, "main_folder is missing")
			return
		}

		// 获取上传文件
		err := r.ParseMultipartForm(32 << 20) //用于解析上传文件，mem：32MB
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		file, handler, err := r.FormFile("file") //获取上传文件
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		logger.LogInfo(logPrefix, "receiving file %s to folder %s/%s", handler.Filename, mainFolder, subFolder)

		// 验证文件类型
		ss := strings.Split(handler.Filename, ".")
		if len(ss) < 2 {
			io.WriteString(w, "file extension name is missing")
			return
		}
		ext := ss[len(ss)-1]
		if !validExt.Contains(ext) {
			io.WriteString(w, fmt.Sprintf("unsupported file ext : .%s", ext))
			return
		}

		// 保存文件
		localPath := ""
		if len(subFolder) == 0 {
			localPath = fmt.Sprintf("%s/%s/%s", rootFolder, mainFolder, handler.Filename)
		} else {
			localPath = fmt.Sprintf("%s/%s/%s/%s", rootFolder, mainFolder, subFolder, handler.Filename)
		}

		if !util.MakeSureDirForFile(localPath) {
			io.WriteString(w, "make sure dir failed")
			return
		}

		if exist, _ := util.PathExists(localPath); exist {
			if os.Remove(localPath) != nil {
				io.WriteString(w, "del old file failed")
				return
			}
		}

		defer file.Close()
		f, err := os.OpenFile(localPath, os.O_WRONLY|os.O_CREATE, 0666) //创建文件
		if err != nil {
			io.WriteString(w, "create new file failed")
			return
		}

		defer f.Close()
		//将上传的文件保存到指定的文件夹中
		if _, err := io.Copy(f, file); err != nil {
			io.WriteString(w, "copy file failed")
			return
		}

		logger.LogInfo(logPrefix, "receive ok")
		io.WriteString(w, "ok")
	} else {
		w.WriteHeader(http.StatusBadRequest)
	}
}
