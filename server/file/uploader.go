/*
 * @Author: aztec
 * @Date: 2023-08-08
 * @Description: 自动同步目录到指定的file service。不支持子目录
 *
 * Copyright (c) 2023 by aztec, All Rights Reserved.
 */

package file

import (
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/logger"
	"github.com/aztecqt/dagger/util/network"
)

type Uploader struct {
	logPrefix        string
	urlUpload        string               // service地址
	urlDelete        string               // service地址
	root             string               // 本地根目录
	remoteMainFolder string               // 上传到远端的那个主目录，必填
	remoteSubFolder  string               // 子目录，选填
	fileModTime      map[string]time.Time // 每个文件的修改时间
	fileStatusPath   string               // 保存文件修改时间的文件
}

func (u *Uploader) Init(url, root, remoteMainFolder, remoteSubFolder string, scanIntervalSec int) {
	u.logPrefix = fmt.Sprintf("uploader-%s", root)
	if len(remoteMainFolder) == 0 {
		logger.LogPanic(u.logPrefix, "missing remoteMainFolder")
	}

	u.urlUpload = url + "/files/upload"
	u.urlDelete = url + "/files/delete"
	u.root = root
	u.remoteMainFolder = remoteMainFolder
	u.remoteSubFolder = remoteSubFolder
	u.fileStatusPath = fmt.Sprintf("%s/file_stat.json", root)
	u.loadFileModTime()

	go func() {
		ticker := time.NewTicker(time.Second * time.Duration(scanIntervalSec))
		for {
			u.scan()
			<-ticker.C
		}
	}()
}

func (u *Uploader) loadFileModTime() {
	util.ObjectFromFile(u.fileStatusPath, &u.fileModTime)
	if u.fileModTime == nil {
		u.fileModTime = make(map[string]time.Time)
	}
}

func (u *Uploader) saveFileModTime() {
	util.ObjectToFile(u.fileStatusPath, u.fileModTime)
}

func (u *Uploader) scan() {
	needSaveModTime := false

	// 遍历根目录下所有文件（除了file_stat.json），获取他们的修改时间
	filepath.Walk(u.root, func(path string, info fs.FileInfo, err error) error {
		// 跳过子目录
		if info.IsDir() {
			return nil
		}

		// 跳过不支持的文件
		ext := util.FileExtName(info.Name())
		if !validExt.Contains(ext) {
			return nil
		}

		// 跳过状态文件
		if info.Name() == "file_stat.json" {
			return nil
		}

		// 判断修改时间
		modTime := info.ModTime()
		needUpload := false
		if lastModTime, ok := u.fileModTime[info.Name()]; ok {
			if modTime.After(lastModTime) {
				u.fileModTime[info.Name()] = modTime
				needSaveModTime = true
				needUpload = true
			}
		} else {
			u.fileModTime[info.Name()] = modTime
			needSaveModTime = true
			needUpload = true
		}

		if needUpload {
			u.upload(info.Name(), path)
		}

		return nil
	})

	// 检查fileStatus里的文件有没有已经不存在的，有的话则删除它
	keys := make([]string, 0, len(u.fileModTime))
	for k := range u.fileModTime {
		keys = append(keys, k)
	}

	for _, fileName := range keys {
		filePath := fmt.Sprintf("%s/%s", u.root, fileName)
		if exist, _ := util.PathExists(filePath); !exist {
			u.delete(fileName)
			delete(u.fileModTime, fileName)
			needSaveModTime = true
		}
	}

	if needSaveModTime {
		u.saveFileModTime()
	}
}

func (u *Uploader) delete(name string) {
	logger.LogInfo(u.logPrefix, "deleting: %s", name)

	params := url.Values{}
	params.Set("file_name", name)
	params.Set("main_folder", u.remoteMainFolder)
	params.Set("sub_folder", u.remoteSubFolder)

	url := u.urlDelete + "?" + params.Encode()
	network.HttpCall(url, "DELETE", "", nil, func(r *http.Response, err error) {
		if err != nil {
			logger.LogInfo(u.logPrefix, "delete %s failed, err=%s", name, err.Error())
		} else {
			body, _ := io.ReadAll(r.Body)
			logger.LogInfo(u.logPrefix, "delete %s result=%s", name, string(body))
		}
	})
}

func (u *Uploader) upload(name, path string) {
	logger.LogInfo(u.logPrefix, "uploading: %s", name)

	bodyBuffer := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuffer)

	fw, e := bodyWriter.CreateFormFile("file", name)
	if e != nil {
		logger.LogImportant(u.logPrefix, e.Error())
		return
	}

	file, e := os.Open(path)
	if e != nil {
		logger.LogImportant(u.logPrefix, e.Error())
		return
	}

	defer file.Close()
	_, e = io.Copy(fw, file)
	if e != nil {
		logger.LogImportant(u.logPrefix, e.Error())
		return
	}

	e = bodyWriter.WriteField("main_folder", u.remoteMainFolder)
	if e != nil {
		logger.LogImportant(u.logPrefix, e.Error())
		return
	}

	e = bodyWriter.WriteField("sub_folder", u.remoteSubFolder)
	if e != nil {
		logger.LogImportant(u.logPrefix, e.Error())
		return
	}

	contentType := bodyWriter.FormDataContentType()
	bodyWriter.Close()

	req, e := http.NewRequest("POST", u.urlUpload, bodyBuffer)
	if e != nil {
		logger.LogImportant(u.logPrefix, e.Error())
		return
	}

	req.Header.Set("Content-Type", contentType)
	client := &http.Client{}
	res, e := client.Do(req)
	if e != nil {
		logger.LogImportant(u.logPrefix, e.Error())
		return
	}

	if res == nil {
		logger.LogImportant(u.logPrefix, "resp is nil")
		return
	}

	if res.Body == nil {
		logger.LogImportant(u.logPrefix, "resp.Body is nil")
		return
	}

	defer res.Body.Close()

	buf, e := io.ReadAll(res.Body)

	if e != nil {
		logger.LogImportant(u.logPrefix, "resp.Body is nil")
		return
	}

	logger.LogInfo(u.logPrefix, "upload result: statusCode=%d, result=%s", res.StatusCode, string(buf))
}
