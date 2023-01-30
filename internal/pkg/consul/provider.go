package consul

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/transcriber"
	tapi "github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/hashicorp/consul/api"
	"go.uber.org/multierr"
)

const (
	uploadKey    = "uploadURL"
	statusKey    = "statusURL"
	resultKey    = "resultURL"
	cleanKey     = "cleanURL"
	isHTTPSSLKey = "HTTPSSL"
)

type Provider struct {
	consul  *api.Client
	srvName string

	lock     *sync.RWMutex
	trans    []*trWrap
	returned int
}

type trWrap struct {
	real tapi.Transcriber
	srv  string
	key  string
}

// NewProvider creates consul service registrator
func NewProvider(cfg *api.Config, srvNameInConsul string) (*Provider, error) {
	c, err := api.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	if srvNameInConsul == "" {
		return nil, fmt.Errorf("no srv name")
	}
	return newProvider(c, srvNameInConsul), nil
}

func newProvider(c *api.Client, srvNameInConsul string) *Provider {
	goapp.Log.Info().Str("service", srvNameInConsul).Msg("cfg: srv name in consul")
	return &Provider{consul: c, srvName: srvNameInConsul, lock: &sync.RWMutex{}, trans: make([]*trWrap, 0)}
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

func (c *Provider) StartRegistryLoop(ctx context.Context, checkInterval time.Duration) (<-chan struct{}, error) {
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
	if err := c.check(ctx); err != nil {
		goapp.Log.Error().Err(err).Send()
	}
	for {
		select {
		case <-ticker.C:
			if err := c.check(ctx); err != nil {
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
	srvs, _, err := c.consul.Health().Service(c.srvName, "", true, (&api.QueryOptions{}).WithContext(ctxInt))
	if err != nil {
		return fmt.Errorf("can't invoke consul: %v", err)
	}
	return c.updateSrv(srvs)
}

func (c *Provider) updateSrv(srvs []*api.ServiceEntry) error {
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
		goapp.Log.Warn().Str("service", s.srv).Msgf("dropped transcriber")
	}
	if len(new) == len(c.trans) && len(ms) == 0 {
		return nil
	}
	c.trans = new
	var err error
	for v, k := range ms {
		tr, errInt := newTranscriber(v, k)
		if errInt != nil {
			err = multierr.Append(err, errInt)
			continue
		}
		c.trans = append(c.trans, tr)
		goapp.Log.Info().Str("service", v).Msg("added transcriber")
	}
	return err
}

func newTranscriber(v string, s *api.ServiceEntry) (*trWrap, error) {
	tr, err := transcriber.NewClient(getUrl(s, uploadKey), getUrl(s, statusKey), getUrl(s, resultKey), getUrl(s, cleanKey))
	if err != nil {
		return nil, fmt.Errorf("can't init transcriber for %s: %v", v, err)
	}
	res := &trWrap{real: tr, srv: v, key: fullKey(s)}
	return res, nil
}

func getUrl(s *api.ServiceEntry, key string) string {
	v, ok := s.Service.Meta[key]
	if !ok {
		return ""
	}
	ssl := ""
	if isSSL, ok := s.Service.Meta[isHTTPSSLKey]; ok {
		if boolValue, err := strconv.ParseBool(isSSL); err == nil && boolValue {
			ssl = "s"
		}
	}
	//todo add https support - see consul HTTP_SSL key
	return fmt.Sprintf("http%s://%s:%d/%s", ssl, s.Service.Address, s.Service.Port, v)
}

func key(s *api.ServiceEntry) string {
	return fmt.Sprintf("%s:%d", s.Service.Address, s.Service.Port)
}

func fullKey(s *api.ServiceEntry) string {
	res := strings.Builder{}
	for _, key := range [...]string{uploadKey, statusKey, resultKey, cleanKey, isHTTPSSLKey} {
		v, ok := s.Service.Meta[key]
		if ok {
			res.WriteString(key + ":" + v + ",")
		}
	}
	return res.String()
}
