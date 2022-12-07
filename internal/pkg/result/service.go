package result

import (
	"context"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"time"

	"github.com/facebookgo/grace/gracehttp"
	"github.com/minio/minio-go/v7"
	"github.com/pkg/errors"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/airenas/roxy/internal/pkg/persistence"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// FileReader loads file by name
type FileReader interface {
	LoadFile(ctx context.Context, name string) (io.ReadSeekCloser, error)
}

// FileNameProvider provides name for result file
type FileNameProvider interface {
	LoadRequest(ctx context.Context, id string) (*persistence.ReqData, error)
}

// Data keeps data required for service work
type Data struct {
	Port         int
	Reader       FileReader
	NameProvider FileNameProvider
}

// StartWebServer starts echo web service
func StartWebServer(data *Data) error {
	goapp.Log.Info().Int("port", data.Port).Msg("Starting BIG TTS Result service")

	if err := validate(data); err != nil {
		return err
	}

	portStr := strconv.Itoa(data.Port)

	e := initRoutes(data)

	e.Server.Addr = ":" + portStr
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 10 * time.Second
	e.Server.WriteTimeout = 5 * time.Minute

	gracehttp.SetLogger(log.New(goapp.Log, "", 0))

	return gracehttp.Serve(e.Server)
}

func validate(data *Data) error {
	if data.Reader == nil {
		return errors.New("no file reader")
	}
	if data.NameProvider == nil {
		return errors.New("no name provider")
	}
	return nil
}

var promMdlw *prometheus.Prometheus

func init() {
	promMdlw = prometheus.NewPrometheus("roxy_result", nil)
}

func initRoutes(data *Data) *echo.Echo {
	e := echo.New()
	e.Use(middleware.Logger())
	promMdlw.Use(e)

	e.GET("/result/:id/:file", download(data))
	e.HEAD("/result/:id/:file", download(data))
	e.GET("/audio/:id", downloadAudio(data))
	e.HEAD("/audio/:id", downloadAudio(data))
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

func download(data *Data) func(echo.Context) error {
	return func(c echo.Context) error {
		defer goapp.Estimate("download method")()

		id := c.Param("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "No ID")
		}
		fileName := c.Param("file")
		if fileName == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "No file")
		}

		fullName, err := url.JoinPath(id, fileName)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusBadRequest, "Wrong name")
		}

		return serveFile(c, data, fullName)
	}
}

func serveFile(c echo.Context, data *Data, name string) error {
	goapp.Log.Info().Str("file", name).Msg("loading")
	file, err := data.Reader.LoadFile(c.Request().Context(), name)
	if err != nil {
		if isNotFound(err) {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		goapp.Log.Error().Err(err).Send()
		return echo.NewHTTPError(http.StatusInternalServerError, "Can't get file")
	}
	defer file.Close()
	stGetter, ok := file.(interface{ Stat() (fs.FileInfo, error) })
	if !ok {
		goapp.Log.Error().Msg(`file does implement "interface{ Stat() (fs.FileInfo, error)"`)
		return echo.NewHTTPError(http.StatusInternalServerError, "Can't get file stat")
	}
	stat, err := stGetter.Stat()
	if err != nil {
		if isNotFound(err) {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusNotFound, "not found")
		}
		goapp.Log.Error().Err(err).Send()
		return echo.NewHTTPError(http.StatusInternalServerError, "Can't get file stat")
	}

	w := c.Response()
	w.Header().Set("Content-Disposition", "attachment; filename="+filepath.Base(stat.Name()))
	http.ServeContent(w, c.Request(), stat.Name(), stat.ModTime(), file)
	return nil
}

func isNotFound(err error) bool {
	var errTest minio.ErrorResponse
	return errors.As(err, &errTest) && errTest.StatusCode == http.StatusNotFound
}

func downloadAudio(data *Data) func(echo.Context) error {
	return func(c echo.Context) error {
		defer goapp.Estimate("download method")()

		id := c.Param("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "No ID")
		}
		req, err := data.NameProvider.LoadRequest(c.Request().Context(), id)
		if err != nil {
			return echo.NewHTTPError(http.StatusBadRequest, "No file by ID")
		}
		fullName, err := url.JoinPath(id, req.Filename)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusBadRequest, "Wrong name")
		}
		return serveFile(c, data, fullName)
	}
}
