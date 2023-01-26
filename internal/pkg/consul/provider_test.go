package consul

import (
	"fmt"
	"testing"

	"github.com/airenas/roxy/internal/pkg/test/mocks"
	tapi "github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/hashicorp/consul/api"
	"github.com/stretchr/testify/assert"
)

func Test_Get_empty(t *testing.T) {
	p := newProvider(nil)
	tr, name, err := p.Get("olia", true)
	assert.Nil(t, tr)
	assert.Equal(t, "", name)
	assert.Nil(t, err)
	tr, name, err = p.Get("olia", false)
	assert.Nil(t, tr)
	assert.Equal(t, "", name)
	assert.NotNil(t, err)
}

func Test_Get_existing(t *testing.T) {
	p := newProvider(nil)
	tr := &mocks.Transcriber{}
	p.trans = append(p.trans, &trWrap{real: tr, srv: "olia"})
	rtr, name, err := p.Get("olia", true)
	assert.Equal(t, tr, rtr)
	assert.Equal(t, "olia", name)
	assert.Nil(t, err)
	rtr, name, err = p.Get("olia1", true)
	assert.Equal(t, tr, rtr)
	assert.Equal(t, "olia", name)
	assert.Nil(t, err)
	rtr, name, err = p.Get("olia", false)
	assert.Equal(t, tr, rtr)
	assert.Equal(t, "olia", name)
	assert.Nil(t, err)
	rtr, name, err = p.Get("olia1", false)
	assert.Nil(t, rtr)
	assert.Equal(t, "", name)
	assert.NotNil(t, err)
}

func Test_Get_by_name(t *testing.T) {
	p := newProvider(nil)
	tr := &mocks.Transcriber{}
	tr1 := &mocks.Transcriber{}
	p.trans = append(p.trans, &trWrap{real: tr, srv: "olia"})
	p.trans = append(p.trans, &trWrap{real: tr1, srv: "olia1"})
	rtr, name, _ := p.Get("olia", true)
	testAssertEqPtr(t, tr, rtr)
	assert.Equal(t, "olia", name)
	rtr, _, _ = p.Get("olia", true)
	testAssertEqPtr(t, tr, rtr)

	rtr, name, _ = p.Get("olia1", true)
	testAssertEqPtr(t, tr1, rtr)
	assert.Equal(t, "olia1", name)
	rtr, _, _ = p.Get("olia1", true)
	testAssertEqPtr(t, tr1, rtr)
}

func Test_Get_round_robin(t *testing.T) {
	p := newProvider(nil)
	tr := &mocks.Transcriber{}
	tr1 := &mocks.Transcriber{}
	p.trans = append(p.trans, &trWrap{real: tr, srv: "olia"})
	p.trans = append(p.trans, &trWrap{real: tr1, srv: "olia1"})
	for i := 0; i < 10; i++ {
		rtr, _, _ := p.Get("", true)
		testAssertEqPtr(t, tr1, rtr)
		rtr, _, _ = p.Get("", true)
		testAssertEqPtr(t, tr, rtr)
	}
}

func testAssertEqPtr(t *testing.T, tr, exp tapi.Transcriber) {
	t.Helper()
	assert.Equal(t, fmt.Sprintf("%p", tr), fmt.Sprintf("%p", exp))
}

func TestProvider_updateSrv_no_meta(t *testing.T) {
	p := newProvider(nil)
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv", Meta: map[string]string{}}}})
	assert.NotNil(t, err)
}

func TestProvider_updateSrv_adds(t *testing.T) {
	p := newProvider(nil)
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
}

func TestProvider_updateSrv_addsSame(t *testing.T) {
	p := newProvider(nil)
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	cp := p.trans[0]
	err = p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	assert.Equal(t, cp, p.trans[0])
}

func TestProvider_updateSrv_updates(t *testing.T) {
	p := newProvider(nil)
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	cp := p.trans[0]
	err = p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{"upload": "upload/", "status": "st", "result": "res", "clean": "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	assert.NotEqual(t, cp, p.trans[0])
}

func TestProvider_updateSrv_addsTwo(t *testing.T) {
	p := newProvider(nil)
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	err = p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 81, Address: "srv",
		Meta: map[string]string{"upload": "upload/", "status": "st", "result": "res", "clean": "cl"}}},
		{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
			Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(p.trans))
}

func TestProvider_updateSrv_drops(t *testing.T) {
	p := newProvider(nil)
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}},
		{Service: &api.AgentService{Service: "olia", Port: 81, Address: "srv",
			Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}},
		{Service: &api.AgentService{Service: "olia", Port: 82, Address: "srv",
			Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 3, len(p.trans))
	c1, c2 := p.trans[0], p.trans[2]
	err = p.updateSrv([]*api.ServiceEntry{
		{Service: &api.AgentService{Service: "olia", Port: 82, Address: "srv",
			Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}},
		{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
			Meta: map[string]string{"upload": "up", "status": "st", "result": "res", "clean": "cl"}}},
	})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(p.trans))
	assert.Equal(t, c1, p.trans[0])
	assert.Equal(t, c2, p.trans[1])
}
