package consul

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/transcriber"
	tapi "github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/hashicorp/consul/api"
)

type Provider struct {
	consul *api.Client

	lock     *sync.RWMutex
	trans    []*trWrap
	returned int
}

type trWrap struct {
	real tapi.Transcriber
	srv  string
	key  string
}

// NewDB creates Request instance
func NewProvider(cfg *api.Config) (*Provider, error) {
	c, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	return &Provider{consul: c, lock: &sync.RWMutex{}, trans: make([]*trWrap, 0)}, nil
}

func (c *Provider) Get(srv string, allowNew bool) (tapi.Transcriber, string, error) {
	c.lock.RLock()
	defer c.lock.RUnlock()
	if !allowNew {
		for _, t := range c.trans {
			if t.srv == srv {
				return t.real, t.srv, nil
			}
		}
		return nil, "", fmt.Errorf("no active srv `%s`", srv)
	}
	// try return same
	for _, t := range c.trans {
		if t.srv == srv {
			return t.real, t.srv, nil
		}
	}
	// else round robin
	c.returned += 1
	if c.returned > len(c.trans)-1 {
		c.returned = 0
	}
	if c.returned < len(c.trans) {
		t := c.trans[c.returned]
		return t.real, t.srv, nil
	}
	return nil, "", nil
}

func (c *Provider) StartCheckLoop(ctx context.Context, checkInterval time.Duration) (<-chan struct{}, error) {
	goapp.Log.Info().Msgf("Starting consul service check every %v", checkInterval)
	res := make(chan struct{}, 2)
	go func() {
		defer close(res)
		c.serviceLoop(ctx, checkInterval)
	}()
	return res, nil
}

func (c *Provider) serviceLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	// run on startup
	err := c.check(ctx)
	if err != nil {
		goapp.Log.Error().Err(err).Send()
	}
	for {
		select {
		case <-ticker.C:
			err := c.check(ctx)
			if err != nil {
				goapp.Log.Error().Err(err).Send()
			}
		case <-ctx.Done():
			ticker.Stop()
			goapp.Log.Info().Msgf("Stopped consul timer service")
			return
		}
	}
}

func (c *Provider) check(ctx context.Context) error {
	ctxInt, cf := context.WithTimeout(ctx, time.Second*5)
	defer cf()
	srvs, _, err := c.consul.Health().Service("asr", "", true, (&api.QueryOptions{}).WithContext(ctxInt))
	if err != nil {
		return fmt.Errorf("can't invoke consul: %v", err)
	}
	goapp.Log.Info().Msgf("got %d services from consul", len(srvs))
	c.lock.Lock()
	defer c.lock.Unlock()
	ms := map[string]*api.ServiceEntry{}
	for _, s := range srvs {
		ms[key(s)] = s
	}
	new := []*trWrap{}
	for _, s := range c.trans {
		if v, ok := ms[s.srv]; ok && s.key == fullKey(v) {
			new = append(new, s)
			delete(ms, s.srv)
			continue
		}
		goapp.Log.Warn().Str("service", s.key).Msgf("dropped transcriber")
	}
	if len(new) == len(c.trans) && len(ms) == 0 {
		return nil
	}
	c.trans = new
	for v, k := range ms {
		tr, err := newTranscriber(v, k)
		if err != nil {
			return err
		}
		c.trans = append(c.trans, tr)
		goapp.Log.Info().Str("service", v).Msg("added transcriber")
	}
	return nil
}

func newTranscriber(v string, s *api.ServiceEntry) (*trWrap, error) {
	tr, err := transcriber.NewClient(getUrl(s, "upload"), getUrl(s, "status"), getUrl(s, "result"), getUrl(s, "clean"))
	if err != nil {
		return nil, fmt.Errorf("can't init transcriber for %s", v)
	}
	res := &trWrap{real: tr, srv: v, key: fullKey(s)}
	return res, nil
}

func getUrl(s *api.ServiceEntry, key string) string {
	v, ok := s.Service.Meta[key]
	if !ok {
		return ""
	}
	return fmt.Sprintf("http://%s:%d/%s", s.Service.Service, s.Service.Port, v)
}

func key(s *api.ServiceEntry) string {
	return fmt.Sprintf("%s:%d", s.Service.Address, s.Service.Port)
}

func fullKey(s *api.ServiceEntry) string {
	res := strings.Builder{}
	for _, key := range [...]string{"upload", "status", "result", "clean"} {
		v, ok := s.Service.Meta[key]
		if ok {
			res.WriteString(key + ":" + v)
		}
	}
	return res.String()
}