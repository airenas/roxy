package transcriber

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"time"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/api"
	tapi "github.com/airenas/roxy/internal/pkg/transcriber/api"
	"github.com/cenkalti/backoff/v4"
	"github.com/gorilla/websocket"
)

// Client comunicates with transcriber service
type Client struct {
	httpclient    *http.Client
	uploadURL     string
	statusWSURL   string
	statusURL     string
	resultURL     string
	cleanURL      string
	uploadTimeout time.Duration
	timeout       time.Duration
	backoff       func() backoff.BackOff
}

// NewClient creates a transcriber client
func NewClient(uploadURL, statusURL, resultURL, cleanURL string) (*Client, error) {
	res := Client{}
	if uploadURL == "" {
		return nil, fmt.Errorf("no uploadURL")
	}
	if statusURL == "" {
		return nil, fmt.Errorf("no statusURL")
	}
	if resultURL == "" {
		return nil, fmt.Errorf("no resultURL")
	}
	if cleanURL == "" {
		return nil, fmt.Errorf("no cleanURL")
	}
	if !strings.HasPrefix(statusURL, "http") {
		return nil, fmt.Errorf("no http in statusURL")
	}
	res.statusWSURL = strings.Replace(statusURL, "http", "ws", 1)
	res.uploadURL = uploadURL
	res.uploadTimeout = time.Minute * 10
	res.statusURL = statusURL
	res.timeout = time.Second * 50
	res.httpclient = asrHTTPClient()
	res.resultURL = resultURL
	res.cleanURL = cleanURL
	res.backoff = newSimpleBackoff
	return &res, nil
}

// HookToStatus to status ws
func (sp *Client) HookToStatus(ctx context.Context, ID string) (<-chan tapi.StatusData, func(), error) {
	goapp.Log.Info().Str("url", sp.statusWSURL).Str("ID", ID).Msg("connect")
	c, err := goapp.InvokeWithBackoff(ctx, func() (*websocket.Conn, bool, error) {
		c, _, err := websocket.DefaultDialer.DialContext(ctx, fmt.Sprintf("%s/subscribe", sp.statusWSURL), nil)
		return c, goapp.IsRetryableErr(err), err
	}, sp.backoff())
	if err != nil {
		return nil, nil, fmt.Errorf("can't dial to status URL: %w", err)
	}
	closeCtx, cf := context.WithCancel(ctx)
	readyCloceCh := make(chan struct{}, 1)
	resF := func() {
		cf()
		select {
		case <-readyCloceCh:
		case <-time.After(time.Second * 5):
		}
		if err = c.Close(); err != nil {
			goapp.Log.Error().Err(err).Msg("socker close error")
		}
	}
	res := make(chan tapi.StatusData, 2)
	go func() {
		defer close(res)
		goapp.Log.Info().Str("ID", ID).Msg("enter status ws read loop")
		for {
			goapp.Log.Debug().Str("ID", ID).Msg("before read")
			_, message, err := c.ReadMessage()
			goapp.Log.Debug().Str("ID", ID).Msg("after read")
			if err != nil {
				goapp.Log.Warn().Err(err).Msg("socker read error")
				break
			}
			var respData tapi.StatusData
			err = json.Unmarshal(message, &respData)
			if err != nil {
				goapp.Log.Error().Err(err).Msg("can't unmarshal status data")
				break
			}
			goapp.Log.Debug().Str("ID", ID).Str("status", respData.Status).Str("error", goapp.Sanitize(respData.Error)).Msg("received status data")
			res <- respData
		}
		goapp.Log.Info().Str("ID", ID).Msg("exit status ws read loop")
	}()
	go func() {
		goapp.Log.Info().Str("ID", ID).Msg("before write ID")
		err := c.WriteMessage(websocket.TextMessage, []byte(ID))
		if err != nil {
			goapp.Log.Error().Err(err).Msg("socker write error")
			return
		}
		goapp.Log.Info().Str("ID", ID).Msg("wrote ID")
		<-closeCtx.Done()
		if err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")); err != nil {
			goapp.Log.Error().Err(err).Msg("socker write error")
		}
		readyCloceCh <- struct{}{}
		goapp.Log.Info().Str("ID", ID).Msg("exit write routine")
	}()
	return res, resF, nil
}

// GetStatus return status by ID
func (sp *Client) GetStatus(ctx context.Context, ID string) (*tapi.StatusData, error) {
	return goapp.InvokeWithBackoff(ctx, func() (*tapi.StatusData, bool, error) {
		ctx, cancelF := context.WithTimeout(ctx, sp.timeout)
		defer cancelF()
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/status/%s", sp.statusURL, ID), nil)
		if err != nil {
			return nil, false, err
		}
		req = req.WithContext(ctx)
		resp, err := sp.httpclient.Do(req)
		if err != nil {
			return nil, goapp.IsRetryableErr(err), fmt.Errorf("can't call: %w", err)
		}
		defer func() {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10000))
			_ = resp.Body.Close()
		}()
		if err := goapp.ValidateHTTPResp(resp, 100); err != nil {
			err = fmt.Errorf("can't invoke '%s': %w", req.URL.String(), err)
			return nil, goapp.IsRetryableCode(resp.StatusCode), err
		}
		res := &tapi.StatusData{}
		err = json.NewDecoder(resp.Body).Decode(&res)
		if err != nil {
			return nil, goapp.IsRetryableErr(err), fmt.Errorf("can't unmarshal: %w", err)
		}
		return res, false, nil
	}, sp.backoff())
}

// GetAudio return initial audio
func (sp *Client) GetAudio(ctx context.Context, ID string) (*tapi.FileData, error) {
	return sp.getFile(ctx, fmt.Sprintf("%s/audio/%s", sp.resultURL, ID))
}

// GetResult return results file
func (sp *Client) GetResult(ctx context.Context, ID, name string) (*tapi.FileData, error) {
	return sp.getFile(ctx, fmt.Sprintf("%s/result/%s/%s", sp.resultURL, ID, name))
}

func (sp *Client) getFile(ctx context.Context, urlStr string) (*tapi.FileData, error) {
	goapp.Log.Info().Str("url", urlStr).Msg("get file")
	return goapp.InvokeWithBackoff(ctx, func() (*tapi.FileData, bool, error) {
		ctx, cancelF := context.WithTimeout(ctx, sp.timeout)
		defer cancelF()
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, false, err
		}
		req = req.WithContext(ctx)
		resp, err := sp.httpclient.Do(req)
		if err != nil {
			return nil, goapp.IsRetryableErr(err), fmt.Errorf("can't call: %w", err)
		}
		defer func() {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10000))
			_ = resp.Body.Close()
		}()
		if err := goapp.ValidateHTTPResp(resp, 100); err != nil {
			err = fmt.Errorf("can't invoke '%s': %w", req.URL.String(), err)
			return nil, goapp.IsRetryableCode(resp.StatusCode), err
		}
		br, err := io.ReadAll(resp.Body)
		if err != nil {
			return nil, goapp.IsRetryableErr(err), fmt.Errorf("can't read body: %w", err)
		}
		res := &tapi.FileData{}
		res.Content = br
		res.Name, err = parseName(resp.Header.Get("content-disposition"))
		if err != nil {
			return nil, false, fmt.Errorf("can't read name: %w", err)
		}
		return res, false, nil
	}, sp.backoff())
}

func parseName(s string) (string, error) {
	_, params, err := mime.ParseMediaType(s)
	if err != nil {
		return "", fmt.Errorf("can't parse header: %w", err)
	}
	return params["filename"], nil
}

type uploadResponse struct {
	ID string `json:"id"`
}

// Upload uploads audio to transcriber service
func (sp *Client) Upload(ctx context.Context, audio *tapi.UploadData) (string, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	i := 0
	for v, k := range audio.Files {
		name := getFileParam(i)
		part, err := writer.CreateFormFile(name, v)
		if err != nil {
			return "", fmt.Errorf("can't add file to request: %w", err)
		}
		_, err = io.Copy(part, k)
		if err != nil {
			return "", fmt.Errorf("can't add file content to request: %w", err)
		}
	}
	for v, k := range audio.Params {
		if err := writer.WriteField(v, k); err != nil {
			return "", fmt.Errorf("can't add param: %w", err)
		}
	}
	writer.Close()

	return goapp.InvokeWithBackoff(ctx, func() (string, bool, error) {
		var respData uploadResponse
		req, err := http.NewRequest(http.MethodPost, sp.uploadURL, body)
		if err != nil {
			return "", false, err
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())

		ctx, cancelF := context.WithTimeout(ctx, sp.uploadTimeout)
		defer cancelF()
		req = req.WithContext(ctx)
		goapp.Log.Info().Str("url", req.URL.String()).Str("method", req.Method).Msg("call")
		resp, err := sp.httpclient.Do(req)
		if err != nil {
			return "", goapp.IsRetryableErr(err), fmt.Errorf("can't call: %w", err)
		}
		defer func() {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10000))
			_ = resp.Body.Close()
		}()
		if err := goapp.ValidateHTTPResp(resp, 100); err != nil {
			err = fmt.Errorf("can't invoke '%s': %w", req.URL.String(), err)
			return "", goapp.IsRetryableCode(resp.StatusCode), err
		}
		br, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", goapp.IsRetryableErr(err), fmt.Errorf("can't read body: %w", err)
		}
		err = json.Unmarshal(br, &respData)
		if err != nil {
			return "", true, fmt.Errorf("can't decode response: %w", err)
		}
		if respData.ID == "" {
			return "", false, fmt.Errorf("can't get ID from response")
		}
		return respData.ID, false, nil
	}, sp.backoff())
}

func getFileParam(i int) string {
	if i == 0 {
		return api.PrmFile
	}
	return fmt.Sprintf("%s%d", api.PrmFile, i+1)
}

// Clean removes all transcription data related with ID
func (sp *Client) Clean(ctx context.Context, ID string) error {
	goapp.Log.Info().Str("url", sp.cleanURL).Msg("delete")
	_, err := goapp.InvokeWithBackoff(ctx,
		func() (interface{}, bool, error) {
			ctx, cancelF := context.WithTimeout(ctx, sp.timeout)
			defer cancelF()
			req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/%s", sp.cleanURL, ID), nil)
			if err != nil {
				return nil, false, err
			}
			req = req.WithContext(ctx)

			resp, err := sp.httpclient.Do(req)
			if err != nil {
				return nil, goapp.IsRetryableErr(err), fmt.Errorf("can't call: %w", err)
			}
			defer func() {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10000))
				_ = resp.Body.Close()
			}()
			if err := goapp.ValidateHTTPResp(resp, 100); err != nil {
				err = fmt.Errorf("can't invoke '%s': %w", req.URL.String(), err)
				return nil, goapp.IsRetryableCode(resp.StatusCode), err
			}
			return nil, false, nil
		}, sp.backoff())
	return err
}

func asrHTTPClient() *http.Client {
	return &http.Client{Transport: newTransport()}
}

func newTransport() http.RoundTripper {
	// default roundripper is not well suited for our case
	// it has just 2 idle connections per host, so try to tune a bit
	res := http.DefaultTransport.(*http.Transport).Clone()
	res.MaxConnsPerHost = 100
	res.MaxIdleConns = 50
	res.MaxIdleConnsPerHost = 50
	res.IdleConnTimeout = 90 * time.Second
	return res
}

func newSimpleBackoff() backoff.BackOff {
	res := backoff.NewExponentialBackOff()
	return backoff.WithMaxRetries(res, 3)
}
