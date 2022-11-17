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
	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/utils"

	"github.com/airenas/go-app/pkg/goapp"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

//FileSaver provides save file functionality
type FileSaver interface {
	Save(name string, r io.Reader) error
}

//MsgSender provides send msg functionality
type MsgSender interface {
	Send(msg amessages.Message, queue, replyQueue string) error
}

//RequestSaver saves requests to DB
type RequestSaver interface {
	SaveRequest(ctx context.Context, req *persistence.ReqData) error
}

// Data keeps data required for service work
type Data struct {
	Port int
	//Configurator *TTSConfigutaror
	Saver     FileSaver
	ReqSaver  RequestSaver
	MsgSender MsgSender
}

const requestIDHEader = "x-doorman-requestid"

//StartWebServer starts echo web service
func StartWebServer(data *Data) error {
	goapp.Log.Infof("Starting HTTP BIG TTS Line service at %d", data.Port)
	if err := validate(data); err != nil {
		return err
	}

	portStr := strconv.Itoa(data.Port)

	e := initRoutes(data)

	e.Server.Addr = ":" + portStr
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 45 * time.Second
	e.Server.WriteTimeout = 30 * time.Second

	w := goapp.Log.Writer()
	defer w.Close()
	gracehttp.SetLogger(log.New(w, "", 0))

	return gracehttp.Serve(e.Server)
}

func validate(data *Data) error {
	// if data.Saver == nil {
	// 	return errors.New("no file saver")
	// }
	if data.ReqSaver == nil {
		return fmt.Errorf("no request saver")
	}
	// if data.MsgSender == nil {
	// 	return errors.New("no msg sender")
	// }
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
	e.GET("/live", live(data))

	goapp.Log.Info("Routes:")
	for _, r := range e.Routes() {
		goapp.Log.Infof("  %s %s", r.Method, r.Path)
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

		err = validateFiles(fHeaders)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, err.Error())
		}

		rd := persistence.ReqData{}
		rd.ID = uuid.New().String()
		rd.Created = time.Now()
		rd.Email = c.FormValue(api.PrmEmail)
		rd.FileCount = len(files)
		rd.Params = takeParams(form)
		rd.Filename = rd.ID + ".mp3"
		rd.AudioReady = false
		if len(files) == 1 {
			ext := filepath.Ext(fHeaders[0].Filename)
			ext = strings.ToLower(ext)
			rd.Filename = rd.ID + ext
			rd.AudioReady = true
		}
		rd.RequestID = extractRequestID(c.Request().Header)
		goapp.Log.Infof("RequestID=%s", goapp.Sanitize(rd.RequestID))

		err = data.ReqSaver.SaveRequest(c.Request().Context(), &rd)
		if err != nil {
			goapp.Log.Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError)
		}

		// err = data.StSaver.Save()
		// if err != nil {
		// 	goapp.Log.Error(err)
		// 	return echo.NewHTTPError(http.StatusInternalServerError)
		// }

		// err = h.data.StatusSaver.SaveF(id, map[string]interface{}{
		// 	"status":                 status.Name(status.Uploaded),
		// 	persistence.StAudioReady: audioReady}, nil)
		// if err != nil {
		// 	http.Error(w, "Can not save status", http.StatusInternalServerError)
		// 	cmdapp.Log.Error(err)
		// 	return
		// }

		err = saveFiles(data.Saver, rd.ID, files, fHeaders)
		if err != nil {
			goapp.Log.Error(err)
			return echo.NewHTTPError(http.StatusInternalServerError)
		}

		// err = h.data.MessageSender.Send(messages.NewQueueMessage(id, recID, tags), msg, "")
		// if err != nil {
		// 	http.Error(w, "Can not send decode message", http.StatusInternalServerError)
		// 	cmdapp.Log.Error(err)
		// 	return
		// }

		res := result{ID: rd.ID}
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

func SumIntsOrFloats[K comparable, V int64 | float64](m map[K]V) V {
	var s V
	for _, v := range m {
		s += v
	}
	return s
}

func extractRequestID(header http.Header) string {
	return header.Get(requestIDHEader)
}

func cleanFiles(f *multipart.Form) {
	if f != nil {
		f.RemoveAll()
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

func validateFiles(fHeaders []*multipart.FileHeader) error {
	for _, h := range fHeaders {
		ext := filepath.Ext(h.Filename)
		if !utils.SupportAudioExt(strings.ToLower(ext)) {
			return errors.New("wrong file extension: " + ext)
		}
		if strings.Contains(h.Filename, "..") {
			return errors.New("wrong file name: " + h.Filename)
		}
	}
	return nil
}

func saveFiles(fs FileSaver, id string, files []multipart.File, fHeaders []*multipart.FileHeader) error {
	if len(files) == 1 {
		ext := filepath.Ext(fHeaders[0].Filename)
		ext = strings.ToLower(ext)
		return fs.Save(id+ext, files[0])
	}

	for i, f := range files {
		fn := fHeaders[i].Filename
		if fn == "" {
			return errors.New("no file name in multipart")
		}
		fn = filepath.Join(id, sanitizeName(fn))
		err := fs.Save(toLowerExt(fn), f)
		if err != nil {
			return errors.Wrapf(err, "can't save %s", fn)
		}
	}
	return nil
}

func sanitizeName(s string) string {
	res := strings.ReplaceAll(s, " ", "_")
	return filepath.Base(res)
}

func toLowerExt(f string) string {
	ext := filepath.Ext(f)
	return f[:len(f)-len(ext)] + strings.ToLower(ext)
}
