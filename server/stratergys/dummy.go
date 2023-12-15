/*
 * @Author: aztec
 * @Date: 2022-10-25 09:15:35
 * @Description: 假的Stratergy对象，以便使各种程序都可以以策略的身份接入CenterServer
 *
 * Copyright (c) 2022 by aztec, All Rights Reserved.
 */
package stratergys

import "github.com/aztecqt/dagger/stratergy"

type DummyStratergy struct {
	simName  string
	simClass string
}

func NewDummyStratergy(name, class string) *DummyStratergy {
	d := new(DummyStratergy)
	d.simName = name
	d.simClass = class
	return d
}

func (d *DummyStratergy) Name() string {
	return d.simName
}

func (d *DummyStratergy) Class() string {
	return d.simClass
}

func (d *DummyStratergy) Status() interface{} {
	return nil
}

func (d *DummyStratergy) Params() *stratergy.Param {
	return nil
}

func (d *DummyStratergy) OnParamChanged(paramData []byte) {

}

func (d *DummyStratergy) OnCommand(cmdLine string, onResp func(string)) {
	onResp("dummy stratergy, no reply...")
}

func (d *DummyStratergy) OnQuantEvent(name string, param map[string]string) bool {
	return false
}

func (d *DummyStratergy) Quit() {

}
