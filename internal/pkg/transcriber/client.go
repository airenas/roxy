package transcriber

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net"
	"net/http"
	"syscall"
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
	goapp.Log.Info().Str("url", sp.statusURL).Str("ID", ID).Msg("connect")
	c, err := invokeWithBackoff(ctx, func() (*websocket.Conn, bool, error) {
		c, _, err := websocket.DefaultDialer.DialContext(ctx, sp.statusURL, nil)
		return c, isRetryable(err), err
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
			goapp.Log.Info().Str("ID", ID).Msg("before read")
			_, message, err := c.ReadMessage()
			goapp.Log.Info().Str("ID", ID).Str("data", string(message)).Msg("after read")
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
			goapp.Log.Debug().Str("ID", ID).Str("data", string(message)).Msg("received status data")
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
	return invokeWithBackoff(ctx, func() (*tapi.FileData, bool, error) {
		ctx, cancelF := context.WithTimeout(ctx, sp.timeout)
		defer cancelF()
		req, err := http.NewRequest(http.MethodGet, urlStr, nil)
		if err != nil {
			return nil, false, err
		}
		req = req.WithContext(ctx)
		resp, err := sp.httpclient.Do(req)
		if err != nil {
			return nil, isRetryable(err), fmt.Errorf("can't call: %w", err)
		}
		defer func() {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10000))
			_ = resp.Body.Close()
		}()
		if err := goapp.ValidateHTTPResp(resp, 100); err != nil {
			err = fmt.Errorf("can't invoke '%s': %w", req.URL.String(), err)
			return nil, isRetryableCode(resp.StatusCode), err
		}
		br, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, isRetryable(err), fmt.Errorf("can't read body: %w", err)
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
		writer.WriteField(v, k)
	}
	writer.Close()

	return invokeWithBackoff(ctx, func() (string, bool, error) {
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
			return "", isRetryable(err), fmt.Errorf("can't call: %w", err)
		}
		defer func() {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10000))
			_ = resp.Body.Close()
		}()
		if err := goapp.ValidateHTTPResp(resp, 100); err != nil {
			err = fmt.Errorf("can't invoke '%s': %w", req.URL.String(), err)
			return "", isRetryableCode(resp.StatusCode), err
		}
		br, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return "", isRetryable(err), fmt.Errorf("can't read body: %w", err)
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
	_, err := invokeWithBackoff(ctx,
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
				return nil, isRetryable(err), fmt.Errorf("can't call: %w", err)
			}
			defer func() {
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10000))
				_ = resp.Body.Close()
			}()
			if err := goapp.ValidateHTTPResp(resp, 100); err != nil {
				err = fmt.Errorf("can't invoke '%s': %w", req.URL.String(), err)
				return nil, isRetryableCode(resp.StatusCode), err
			}
			return nil, false, nil
		}, sp.backoff())
	return err
}

func isRetryableCode(c int) bool {
	return c != http.StatusBadRequest && c != http.StatusUnauthorized && c != http.StatusNotFound && c != http.StatusConflict
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

func invokeWithBackoff[K any](ctx context.Context, f func() (K, bool, error), b backoff.BackOff) (K, error) {
	c := 0
	op := func() (K, error) {
		select {
		case <-ctx.Done():
			return *new(K), backoff.Permanent(context.DeadlineExceeded)
		default:
			if c > 0 {
				goapp.Log.Info().Int("count", c).Msg("retry")
			}
		}
		c++
		res, retry, err := f()
		if err != nil && !retry {
			goapp.Log.Info().Msg("not retryable error")
			return res, backoff.Permanent(err)
		}
		return res, err
	}
	res, err := backoff.RetryWithData(op, newSimpleBackoff())
	return res, err
}

func isRetryable(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		isTimeout(err)
}

func isTimeout(err error) bool {
	e, ok := err.(net.Error)
	return ok && e.Timeout()
}

func newSimpleBackoff() backoff.BackOff {
	res := backoff.NewExponentialBackOff()
	return backoff.WithMaxRetries(res, 3)
}
