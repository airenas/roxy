package clean

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/facebookgo/grace/gracehttp"
	"github.com/pkg/errors"

	"github.com/airenas/go-app/pkg/goapp"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// Cleaner is a wrapper for clean functionality
type Cleaner interface {
	Clean(ctx context.Context, ID string) error
}

// Data keeps data required for service work
type Data struct {
	Port    int
	Cleaner Cleaner
}

// StartWebServer starts echo web service
func StartWebServer(data *Data) error {
	goapp.Log.Info().Int("port", data.Port).Msgf("Starting HTTP ROXY clean service")
	if err := validate(data); err != nil {
		return err
	}

	portStr := strconv.Itoa(data.Port)

	e := initRoutes(data)

	e.Server.Addr = ":" + portStr
	e.Server.ReadHeaderTimeout = 5 * time.Second
	e.Server.ReadTimeout = 10 * time.Second
	e.Server.WriteTimeout = 10 * time.Second

	gracehttp.SetLogger(log.New(goapp.Log, "", 0))

	return gracehttp.Serve(e.Server)
}

func validate(data *Data) error {
	if data.Cleaner == nil {
		return errors.New("no cleaner")
	}
	return nil
}

var promMdlw *prometheus.Prometheus

func init() {
	promMdlw = prometheus.NewPrometheus("roxy_clean", nil)
}

func initRoutes(data *Data) *echo.Echo {
	e := echo.New()
	e.Use(middleware.Logger())
	promMdlw.Use(e)

	e.DELETE("/delete/:id", delete(data.Cleaner))
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

func delete(cleaner Cleaner) func(echo.Context) error {
	return func(c echo.Context) error {
		defer goapp.Estimate("delete method")()

		id := c.Param("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "No ID")
		}
		err := cleaner.Clean(c.Request().Context(), id)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusInternalServerError, "Can't delete")
		}
		return c.String(http.StatusOK, "deleted")
	}
}
