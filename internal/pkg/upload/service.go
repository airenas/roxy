package upload

import (
	"context"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/facebookgo/grace/gracehttp"
	"github.com/google/uuid"
	"github.com/pkg/errors"

	amessages "github.com/airenas/async-api/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/api"
	"github.com/airenas/roxy/internal/pkg/messages"
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/status"
	"github.com/airenas/roxy/internal/pkg/utils"

	"github.com/airenas/go-app/pkg/goapp"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// FileSaver provides save file functionality
type FileSaver interface {
	SaveFile(ctx context.Context, name string, r io.Reader, fileSize int64) error
}

// MsgSender provides send msg functionality
type MsgSender interface {
	SendMessage(context.Context, amessages.Message, string) error
}

// DBSaver saves requests to DB
type DB interface {
	InsertRequest(ctx context.Context, req *persistence.ReqData) error
	InsertStatus(ctx context.Context, req *persistence.Status) error
	DeleteWorkData(ctx context.Context, ID string) error
}

// Data keeps data required for service work
type Data struct {
	Port        int
	Saver       FileSaver
	DB          DB
	MsgSender   MsgSender
	RetrySecret string
}

const requestIDHEader = "x-doorman-requestid"

// StartWebServer starts echo web service
func StartWebServer(data *Data) error {
	goapp.Log.Info().Msgf("Starting HTTP ROXY upload service at %d", data.Port)
	if err := validate(data); err != nil {
		return err
	}

	portStr := strconv.Itoa(data.Port)

	e := initRoutes(data)

	e.Server.Addr = ":" + portStr
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 180 * time.Second
	e.Server.WriteTimeout = 30 * time.Second

	gracehttp.SetLogger(log.New(goapp.Log, "", 0))

	return gracehttp.Serve(e.Server)
}

func validate(data *Data) error {
	if data.Saver == nil {
		return errors.New("no file saver")
	}
	if data.DB == nil {
		return fmt.Errorf("no DB")
	}
	if data.MsgSender == nil {
		return fmt.Errorf("no msg sender")
	}
	return nil
}

var promMdlw *prometheus.Prometheus

func init() {
	promMdlw = prometheus.NewPrometheus("roxy_upload", nil)
}

func initRoutes(data *Data) *echo.Echo {
	e := echo.New()
	e.Use(middleware.Logger())
	promMdlw.Use(e)

	e.POST("/upload", upload(data))
	if data.RetrySecret != "" {
		e.POST(fmt.Sprintf("/retry/%s/:id", data.RetrySecret), retry(data))
	}
	e.GET("/live", live(data))

	goapp.Log.Info().Msg("Routes:")
	for _, r := range e.Routes() {
		goapp.Log.Info().Msgf("  %s %s", r.Method, r.Path)
	}
	return e
}

func live(data *Data) func(echo.Context) error {
	return func(c echo.Context) error {
		return c.JSONBlob(http.StatusOK, []byte(`{"service":"OK"}`))
	}
}

type result struct {
	ID string `json:"id"`
}

func upload(data *Data) func(echo.Context) error {
	return func(c echo.Context) error {
		defer goapp.Estimate("upload method")()
		ctx := c.Request().Context()

		form, err := c.MultipartForm()
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "no multipart form data")
		}
		defer cleanFiles(form)
		err = validateFormParams(form)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		files, fHeaders, err := takeFiles(form, api.PrmFile)
		for _, f := range files {
			fInt := f
			defer fInt.Close()
		}
		if err != nil && len(files) == 0 {
			return echo.NewHTTPError(http.StatusBadRequest, "no file")
		}
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "wrong input form")
		}

		rd := persistence.ReqData{}
		rd.ID = uuid.New().String()
		rd.FileNames, err = validateExtractFiles(fHeaders)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		rd.Created = time.Now()
		rd.Email = utils.ToSQLStr(c.FormValue(api.PrmEmail))
		rd.FileCount = len(files)
		rd.Params = takeParams(form)
		rd.FileName = utils.ToSQLStr(rd.ID + ".mp3")
		audioReady := false
		if len(files) == 1 {
			rd.FileName = utils.ToSQLStr(rd.FileNames[0])
			audioReady = true
		}
		rd.RequestID = extractRequestID(c.Request().Header)
		goapp.Log.Info().Str("requestID", rd.RequestID).Msg("request info")

		err = data.DB.InsertRequest(ctx, &rd)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusInternalServerError)
		}
		err = data.DB.InsertStatus(ctx, &persistence.Status{ID: rd.ID, Status: status.Uploaded.String(),
			Created: time.Now(), AudioReady: audioReady})
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusInternalServerError)
		}
		err = saveFiles(ctx, data.Saver, rd.ID, files, fHeaders)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusInternalServerError)
		}
		err = data.MsgSender.SendMessage(ctx, &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: rd.ID}}, messages.Upload)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusInternalServerError)
		}

		res := result{ID: rd.ID}
		return c.JSON(http.StatusOK, res)
	}
}

func retry(data *Data) func(echo.Context) error {
	return func(c echo.Context) error {
		defer goapp.Estimate("retry method")()
		ctx := c.Request().Context()
		id := c.Param("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "No ID")
		}
		err := data.DB.DeleteWorkData(ctx, id)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusInternalServerError)
		}
		err = data.MsgSender.SendMessage(ctx, &messages.ASRMessage{QueueMessage: amessages.QueueMessage{ID: id}}, messages.Upload)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusInternalServerError)
		}

		res := result{ID: id}
		return c.JSON(http.StatusOK, res)
	}
}

func takeParams(form *multipart.Form) map[string]string {
	res := map[string]string{}
	for k, v := range form.Value {
		res[k] = takeFirst(v, "")
	}
	return res
}

func takeFirst[K interface{}](a []K, d K) K {
	if len(a) > 0 {
		return a[0]
	}
	return d
}

func extractRequestID(header http.Header) string {
	return header.Get(requestIDHEader)
}

func cleanFiles(f *multipart.Form) {
	if f != nil {
		_ = f.RemoveAll()
	}
}

func validateFormParams(form *multipart.Form) error {
	allowed := map[string]bool{api.PrmEmail: true, api.PrmRecognizer: true,
		api.PrmNumberOfSpeakers: true, api.PrmSkipNumJoin: true, api.PrmSepSpeakersOnChannel: true}
	for k := range form.Value {
		_, f := allowed[k]
		if !f {
			return errors.Errorf("unknown parameter '%s'", k)
		}
	}
	return validateFormFiles(form)
}

func validateFormFiles(form *multipart.Form) error {
	check := make(map[string]bool)
	if form != nil {
		for k := range form.File {
			check[k] = true
		}
	}
	if !check[api.PrmFile] {
		return errors.New("no form file parameter 'file'")
	}
	delete(check, api.PrmFile)
	for i := 2; i <= 10; i++ {
		pn := api.PrmFile + strconv.Itoa(i)
		if !check[pn] {
			break
		}
		delete(check, pn)
	}
	for k := range check {
		return errors.Errorf("unexpected form file parameters '%v'", k)
	}
	return nil
}

func takeFiles(form *multipart.Form, paramName string) ([]multipart.File, []*multipart.FileHeader, error) {
	file, handler, err := takeFile(form, paramName)
	if err != nil {
		return nil, nil, fmt.Errorf("no form param file: %w", err)
	}
	fRes := []multipart.File{file}
	fhRes := []*multipart.FileHeader{handler}
	for i := 2; i <= 10; i++ {
		file, handler, err := takeFile(form, paramName+strconv.Itoa(i))
		if err == http.ErrMissingFile {
			break
		}
		if err != nil {
			return fRes, nil, fmt.Errorf("error reading form param '%s' : %w", paramName+strconv.Itoa(i), err)
		}
		fRes = append(fRes, file)
		fhRes = append(fhRes, handler)
	}
	return fRes, fhRes, nil
}

func takeFile(form *multipart.Form, paramName string) (multipart.File, *multipart.FileHeader, error) {
	handler := takeFirst(form.File[paramName], nil)
	if handler == nil {
		return nil, nil, http.ErrMissingFile
	}
	file, err := handler.Open()
	return file, handler, err
}

func validateExtractFiles(fHeaders []*multipart.FileHeader) ([]string, error) {
	res := []string{}
	for _, h := range fHeaders {
		ext := filepath.Ext(h.Filename)
		if !utils.SupportAudioExt(strings.ToLower(ext)) {
			return nil, fmt.Errorf("wrong file extension: " + ext)
		}
		fn, err := utils.MakeValidateFileName("", h.Filename)
		if err != nil {
			return nil, fmt.Errorf("wrong file name: " + h.Filename)
		}
		res = append(res, fn)
	}
	return res, nil
}

func saveFiles(ctx context.Context, fs FileSaver, id string, files []multipart.File, fHeaders []*multipart.FileHeader) error {
	for i, f := range files {
		if fHeaders[i].Filename == "" {
			return errors.New("no file name in multipart")
		}
		fn, err := utils.MakeValidateFileName(id, fHeaders[i].Filename)
		if err != nil {
			return fmt.Errorf("can't save '%s': %w", fHeaders[i].Filename, err)
		}
		if err = fs.SaveFile(ctx, fn, f, fHeaders[i].Size); err != nil {
			return fmt.Errorf("can't save '%s': %w", fn, err)
		}
	}
	return nil
}
