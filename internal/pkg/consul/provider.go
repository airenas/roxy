package consul

import (
	"context"
	"fmt"
	"math/rand"
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
	priorityKey  = "priority"
)

type Provider struct {
	consul  *api.Client
	srvName string

	lock  *sync.RWMutex
	trans []*trWrap
}

type trWrap struct {
	real     tapi.Transcriber
	srv      string
	key      string
	priority float64
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
	if len(c.trans) == 0 {
		return nil, "", nil
	}
	// try return same
	for _, t := range c.trans {
		if t.srv == srv {
			return t.real, t.srv, nil
		}
	}
	if len(c.trans) == 1 {
		t := c.trans[0]
		return t.real, t.srv, nil
	}
	// else random select by priority
	i, err := getRandomByPriority(c.trans)
	if err != nil {
		return nil, "", fmt.Errorf("can't select recognizer: %v", err)
	}
	if i < len(c.trans) {
		t := c.trans[i]
		return t.real, t.srv, nil
	}
	return nil, "", nil
}

func getRandomByPriority(trWraps []*trWrap) (int, error) {
	prMax := 0.0
	for _, tr := range trWraps {
		prMax += tr.priority
	}
	if prMax < 0.1 {
		return 0, fmt.Errorf("wrong priority sum found %f", prMax)
	}
	rnd := rand.Float64() * prMax
	prMax = 0.0
	for i, tr := range trWraps {
		prMax += tr.priority
		if prMax > rnd {
			return i, nil
		}
	}
	return len(trWraps), nil
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
		goapp.Log.Info().Str("service", v).Float64("priority", tr.priority).Msg("added transcriber")
	}
	return err
}

func newTranscriber(v string, s *api.ServiceEntry) (*trWrap, error) {
	tr, err := transcriber.NewClient(getUrl(s, uploadKey), getUrl(s, statusKey), getUrl(s, resultKey), getUrl(s, cleanKey))
	if err != nil {
		return nil, fmt.Errorf("can't init transcriber for %s: %v", v, err)
	}
	priority, err := getPriority(s)
	if err != nil {
		return nil, fmt.Errorf("can't init transcriber for %s: %v", v, err)
	}
	res := &trWrap{real: tr, srv: v, key: fullKey(s), priority: priority}
	return res, nil
}

func getPriority(s *api.ServiceEntry) (float64, error) {
	v, ok := s.Service.Meta[priorityKey]
	if !ok {
		return 1, nil
	}
	res, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("can't parse priority '%s': %v", v, err)
	}
	if res < 0.5 || res > 50 {
		return 0, fmt.Errorf("wrong priority value '%f', not in [0.5, 50]", res)
	}
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
	for _, key := range [...]string{uploadKey, statusKey, resultKey, cleanKey, isHTTPSSLKey, priorityKey} {
		v, ok := s.Service.Meta[key]
		if ok {
			res.WriteString(key + ":" + v + ",")
		}
	}
	return res.String()
}
