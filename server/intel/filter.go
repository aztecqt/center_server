/*
 * @Author: aztec
 * @Date: 2022-9-16 16:20
 * @Description: 钉钉通知的过滤器，记录用户的订阅信息，订阅信息需要持久化
 *
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */

package intel

import (
	"fmt"
	"strings"
	"sync"

	"github.com/aztecqt/dagger/util"
	"github.com/aztecqt/dagger/util/logger"
)

// 用户的订阅过滤数据
type dingUserTypeFilter struct {
	Nick           string                            `json:"nick"`
	SubTypeFilters map[string]*dingUserSubtypeFilter `json:"types"`
}

func newDingUserTypeFilter() *dingUserTypeFilter {
	f := new(dingUserTypeFilter)
	f.SubTypeFilters = make(map[string]*dingUserSubtypeFilter)
	return f
}

func (d *dingUserTypeFilter) match(mainType, subType string) bool {
	if stf, ok := d.SubTypeFilters[mainType]; ok {
		if _, ok := stf.WlSubtypes[subType]; !ok {
			if len(stf.WlSubtypes) > 0 {
				return false // 白名单非空，且不包含该Subtype
			}
		}

		if _, ok := stf.BlSubtypes[subType]; ok {
			return false // 黑名单包含该subtype
		}

		return true
	} else {
		return false
	}
}

// 子类型过滤数据
type dingUserSubtypeFilter struct {
	WlSubtypes map[string]int `json:"white_list"` // 子类型白名单。为空则表示全部允许
	BlSubtypes map[string]int `json:"black_list"` // 子类型黑名单
}

func newDingUserSubtypeFilter() *dingUserSubtypeFilter {
	f := new(dingUserSubtypeFilter)
	f.WlSubtypes = make(map[string]int)
	f.BlSubtypes = make(map[string]int)
	return f
}

// 主过滤器
type dingFilter struct {
	// 所有用户的订阅数据，以uid作为key
	UserTypeFilters map[string]*dingUserTypeFilter `json:"user_filter"`

	// 线程安全
	mu sync.RWMutex
}

// 初始化
func (df *dingFilter) init() {
	df.UserTypeFilters = make(map[string]*dingUserTypeFilter)
	df.fromFile()
}

// 寻找符合条件的用户id列表
func (df *dingFilter) findMatchedUsers(mainType, subType string) []string {
	uids := make([]string, 0)
	for uid, dutf := range df.UserTypeFilters {
		if dutf.match(mainType, subType) {
			uids = append(uids, uid)
		}
	}
	return uids
}

// 保存文件
func (df *dingFilter) toFile() {
	if !util.ObjectToFile("ding_filter.json", df) {
		logger.LogImportant(logPrefix, "save ding_filter.json failed")
	} else {
		logger.LogImportant(logPrefix, "save ding_filter.json ok")
	}
}

// 加载
func (df *dingFilter) fromFile() {
	if !util.ObjectFromFile("ding_filter.json", df) {
		logger.LogImportant(logPrefix, "load ding_filter.json failed")
	} else {
		logger.LogImportant(logPrefix, "load ding_filter.json ok")
	}
}

// 查询某人的订阅情况
func (df *dingFilter) userFilter(uid string) *dingUserTypeFilter {
	if d, ok := df.UserTypeFilters[uid]; ok {
		return d
	} else {
		return nil
	}
}

// 返回某用户的过滤器详情
func (df *dingFilter) userFilterStr(uid string) string {
	ss := strings.Builder{}
	f := df.userFilter(uid)
	if f == nil {
		ss.WriteString("nothing")
	} else {
		ss.WriteString(fmt.Sprintf("用户昵称:[%s]\n", f.Nick))
		ss.WriteString(fmt.Sprintf("已订阅内容:\n"))
		for k, dusf := range f.SubTypeFilters {
			ss.WriteString(fmt.Sprintf("*[%s]\n", k))
			wlTypes := make([]string, 0, len(dusf.WlSubtypes))
			for k2 := range dusf.WlSubtypes {
				wlTypes = append(wlTypes, k2)
			}
			blTypes := make([]string, 0, len(dusf.BlSubtypes))
			for k2 := range dusf.BlSubtypes {
				blTypes = append(blTypes, k2)
			}
			if len(wlTypes) > 0 {
				ss.WriteString(fmt.Sprintf("  +%s\n", strings.Join(wlTypes, ",")))
			}
			if len(blTypes) > 0 {
				ss.WriteString(fmt.Sprintf("  -%s\n", strings.Join(blTypes, ",")))
			}
		}
	}
	return ss.String()
}

// 查询某人的过滤器，没有则创建
func (df *dingFilter) findOrCreateUserFilter(uid string) *dingUserTypeFilter {
	var userFilter *dingUserTypeFilter
	if _, ok := df.UserTypeFilters[uid]; !ok {
		userFilter = newDingUserTypeFilter()
		df.UserTypeFilters[uid] = userFilter
	} else {
		userFilter = df.UserTypeFilters[uid]
	}

	return userFilter
}

// 查询某人某类型的子类型订阅情况，没有则创建
func (df *dingFilter) findOrCreateUserSubtypeFilter(uid, mainType string) *dingUserSubtypeFilter {
	typeFilter := df.findOrCreateUserFilter(uid)
	var subtypeFilter *dingUserSubtypeFilter
	if _, ok := typeFilter.SubTypeFilters[mainType]; !ok {
		subtypeFilter = newDingUserSubtypeFilter()
		typeFilter.SubTypeFilters[mainType] = subtypeFilter
	} else {
		subtypeFilter = typeFilter.SubTypeFilters[mainType]
	}
	return subtypeFilter
}

// 订阅某类型
func (df *dingFilter) subscribeType(uid, nick, mainType string) {
	df.mu.Lock()
	defer df.mu.Unlock()
	mainType = strings.ToLower(mainType)

	filter := df.findOrCreateUserFilter(uid)
	filter.Nick = nick
	df.findOrCreateUserSubtypeFilter(uid, mainType)
	df.toFile()
}

// 反订阅某类型
func (df *dingFilter) unsubscribeType(uid, nick, mainType string) {
	df.mu.Lock()
	defer df.mu.Unlock()
	mainType = strings.ToLower(mainType)

	filter := df.findOrCreateUserFilter(uid)
	filter.Nick = nick
	delete(filter.SubTypeFilters, mainType)
	df.toFile()
}

// 子类型加入白名单
func (df *dingFilter) userAddSubtypeWhiteList(uid, mainType, subType string) {
	df.mu.Lock()
	defer df.mu.Unlock()
	mainType = strings.ToLower(mainType)
	subType = strings.ToLower(subType)

	subFilter := df.findOrCreateUserSubtypeFilter(uid, mainType)
	subFilter.WlSubtypes[subType] = 0
	df.toFile()
}

// 子类型移出白名单
func (df *dingFilter) userRemoveSubtypeWhiteList(uid, mainType, subType string) {
	df.mu.Lock()
	defer df.mu.Unlock()
	mainType = strings.ToLower(mainType)
	subType = strings.ToLower(subType)

	subFilter := df.findOrCreateUserSubtypeFilter(uid, mainType)
	delete(subFilter.WlSubtypes, subType)
	df.toFile()
}

// 清空白名单
func (df *dingFilter) userClearSubtypeWhiteList(uid, mainType string) {
	df.mu.Lock()
	defer df.mu.Unlock()
	mainType = strings.ToLower(mainType)

	subFilter := df.findOrCreateUserSubtypeFilter(uid, mainType)
	subFilter.WlSubtypes = make(map[string]int)
	df.toFile()
}

// 子类型加入黑名单
func (df *dingFilter) userAddSubtypeBlackList(uid, mainType, subType string) {
	df.mu.Lock()
	defer df.mu.Unlock()
	mainType = strings.ToLower(mainType)
	subType = strings.ToLower(subType)

	subFilter := df.findOrCreateUserSubtypeFilter(uid, mainType)
	subFilter.BlSubtypes[subType] = 0
	df.toFile()
}

// 子类型移出黑名单
func (df *dingFilter) userRemoveSubtypeBlackList(uid, mainType, subType string) {
	df.mu.Lock()
	defer df.mu.Unlock()
	mainType = strings.ToLower(mainType)
	subType = strings.ToLower(subType)

	subFilter := df.findOrCreateUserSubtypeFilter(uid, mainType)
	delete(subFilter.BlSubtypes, subType)
	df.toFile()
}

// 清空黑名单
func (df *dingFilter) userClearSubtypeBlackList(uid, mainType string) {
	df.mu.Lock()
	defer df.mu.Unlock()
	mainType = strings.ToLower(mainType)

	subFilter := df.findOrCreateUserSubtypeFilter(uid, mainType)
	subFilter.BlSubtypes = make(map[string]int)
	df.toFile()
}
