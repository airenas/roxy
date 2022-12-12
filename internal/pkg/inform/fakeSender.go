package inform

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/jordan-wright/email"
	"github.com/spf13/viper"
)

// fakeEmailSender uses http url to call instead of real sending msg
type fakeEmailSender struct {
	url string
}

// NewFakeEmailSender initiates email sender
func NewFakeEmailSender(c *viper.Viper) (*fakeEmailSender, error) {
	r := fakeEmailSender{}
	r.url = c.GetString("smtp.fakeUrl")
	if r.url == "" {
		return nil, fmt.Errorf("no URL")
	}
	goapp.Log.Info().Str("URL", r.url).Msgf("Fake sender")
	return &r, nil
}

// Send sends email
func (s *fakeEmailSender) Send(email *email.Email) error {
	body, err := json.Marshal(email)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, s.url, bytes.NewReader(body))
	if err != nil {
		return err
	}

	ctx, cancelF := context.WithTimeout(context.TODO(), time.Second*5)
	defer cancelF()
	req = req.WithContext(ctx)
	goapp.Log.Info().Str("url", req.URL.String()).Str("method", req.Method).Msg("call")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10000))
		_ = resp.Body.Close()
	}()
	if err := goapp.ValidateHTTPResp(resp, 100); err != nil {
		err = fmt.Errorf("can't invoke '%s': %w", req.URL.String(), err)
		return err
	}
	return nil
}
