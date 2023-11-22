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
	p := newProvider(nil, "")
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
	p := newProvider(nil, "")
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
	p := newProvider(nil, "")
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

func Test_Get_by_priority(t *testing.T) {
	p := newProvider(nil, "")
	tr := &mocks.Transcriber{}
	tr1 := &mocks.Transcriber{}
	p.trans = append(p.trans, &trWrap{real: tr, srv: "olia", priority: 8})
	p.trans = append(p.trans, &trWrap{real: tr1, srv: "olia1", priority: 2})
	counts := map[tapi.Transcriber]int{}
	for i := 0; i < 1000; i++ {
		rtr, _, _ := p.Get("", true)
		counts[rtr] = counts[rtr] + 1
	}
	assert.GreaterOrEqual(t, counts[tr], 700)
	assert.GreaterOrEqual(t, counts[tr1], 100)
	assert.LessOrEqual(t, counts[tr], 900)
	assert.LessOrEqual(t, counts[tr1], 300)
}

func Test_Get_by_priority2(t *testing.T) {
	p := newProvider(nil, "")
	tr := &mocks.Transcriber{}
	tr1 := &mocks.Transcriber{}
	p.trans = append(p.trans, &trWrap{real: tr, srv: "olia", priority: 9.5})
	p.trans = append(p.trans, &trWrap{real: tr1, srv: "olia1", priority: 0.5})
	counts := map[tapi.Transcriber]int{}
	for i := 0; i < 1000; i++ {
		rtr, _, _ := p.Get("", true)
		counts[rtr] = counts[rtr] + 1
	}
	assert.GreaterOrEqual(t, counts[tr], 920)
	assert.GreaterOrEqual(t, counts[tr1], 20)
	assert.LessOrEqual(t, counts[tr], 980)
	assert.LessOrEqual(t, counts[tr1], 70)
}

func Test_Get_by_priority_empty(t *testing.T) {
	p := newProvider(nil, "")
	rtr, _, err := p.Get("", true)
	assert.Nil(t, err)
	assert.Nil(t, rtr)
}

func Test_Get_by_priority_one(t *testing.T) {
	p := newProvider(nil, "")
	tr := &mocks.Transcriber{}
	p.trans = append(p.trans, &trWrap{real: tr, srv: "olia", priority: 9.5})
	rtr, _, err := p.Get("", true)
	assert.Nil(t, err)
	assert.NotNil(t, rtr)
}

func testAssertEqPtr(t *testing.T, tr, exp tapi.Transcriber) {
	t.Helper()
	assert.Equal(t, fmt.Sprintf("%p", tr), fmt.Sprintf("%p", exp))
}

func TestProvider_updateSrv_no_meta(t *testing.T) {
	p := newProvider(nil, "")
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv", Meta: map[string]string{}}}})
	assert.NotNil(t, err)
}

func TestProvider_updateSrv_adds(t *testing.T) {
	p := newProvider(nil, "")
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
}

func TestProvider_updateSrv_addsSame(t *testing.T) {
	p := newProvider(nil, "")
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	cp := p.trans[0]
	err = p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	assert.Equal(t, cp, p.trans[0])
}

func TestProvider_updateSrv_updates(t *testing.T) {
	p := newProvider(nil, "")
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	cp := p.trans[0]
	err = p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{uploadKey: "upload/", statusKey: "st", resultKey: "res", cleanKey: "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	assert.NotEqual(t, cp, p.trans[0])
}

func TestProvider_updateSrv_addsTwo(t *testing.T) {
	p := newProvider(nil, "")
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 1, len(p.trans))
	err = p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 81, Address: "srv",
		Meta: map[string]string{uploadKey: "upload/", statusKey: "st", resultKey: "res", cleanKey: "cl"}}},
		{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
			Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(p.trans))
}

func TestProvider_updateSrv_drops(t *testing.T) {
	p := newProvider(nil, "")
	err := p.updateSrv([]*api.ServiceEntry{{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
		Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}},
		{Service: &api.AgentService{Service: "olia", Port: 81, Address: "srv",
			Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}},
		{Service: &api.AgentService{Service: "olia", Port: 82, Address: "srv",
			Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}}})
	assert.Nil(t, err)
	assert.Equal(t, 3, len(p.trans))

	gt := index(p.trans, "srv:80") > index(p.trans, "srv:82")
	err = p.updateSrv([]*api.ServiceEntry{
		{Service: &api.AgentService{Service: "olia", Port: 82, Address: "srv",
			Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}},
		{Service: &api.AgentService{Service: "olia", Port: 80, Address: "srv",
			Meta: map[string]string{uploadKey: "up", statusKey: "st", resultKey: "res", cleanKey: "cl"}}},
	})
	assert.Nil(t, err)
	assert.Equal(t, 2, len(p.trans))
	assert.Equal(t, gt, index(p.trans, "srv:80") > index(p.trans, "srv:82"))
}

func index(trWrap []*trWrap, s string) int {
	for i, tr := range trWrap {
		if tr.srv == s {
			return i
		}
	}
	return -1
}

func Test_getUrl(t *testing.T) {
	type args struct {
		s   *api.ServiceEntry
		key string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "http", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr"}}}, key: "uploadURL"},
			want: "http://srv:81/upload/tr"},
		{name: "https", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr", "HTTPSSL": "1"}}}, key: "uploadURL"},
			want: "https://srv:81/upload/tr"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getUrl(tt.args.s, tt.args.key); got != tt.want {
				t.Errorf("getUrl() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getPriority(t *testing.T) {
	type args struct {
		s *api.ServiceEntry
	}
	tests := []struct {
		name    string
		args    args
		want    float64
		wantErr bool
	}{
		{name: "no", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr"}}}},
			want: 1, wantErr: false},
		{name: "extract", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr", "priority": "10"}}}},
			want: 10, wantErr: false},
		{name: "extract", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr", "priority": "10.5"}}}},
			want: 10.5, wantErr: false},
		{name: "fails", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr", "priority": "aa10.5"}}}},
			want: 0, wantErr: true},
		{name: "too low", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr", "priority": "0.4"}}}},
			want: 0, wantErr: true},
		{name: "too big", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr", "priority": "100.5"}}}},
			want: 0, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := getPriority(tt.args.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("getPriority() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("getPriority() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_fullKey(t *testing.T) {
	type args struct {
		s *api.ServiceEntry
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{name: "priority", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr", "priority": "100.5"}}}},
			want: "uploadURL:upload/tr,priority:100.5,"},
		{name: "no priority", args: args{s: &api.ServiceEntry{Service: &api.AgentService{Address: "srv", Port: 81, Meta: map[string]string{"uploadURL": "upload/tr"}}}},
			want: "uploadURL:upload/tr,"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fullKey(tt.args.s); got != tt.want {
				t.Errorf("fullKey() = %v, want %v", got, tt.want)
			}
		})
	}
}
