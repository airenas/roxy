package statusservice

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/facebookgo/grace/gracehttp"
	"github.com/gorilla/websocket"

	"github.com/airenas/roxy/internal/pkg/persistence"
	"github.com/airenas/roxy/internal/pkg/utils"

	"github.com/airenas/go-app/pkg/goapp"

	"github.com/labstack/echo-contrib/prometheus"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// DB loads status info
type DB interface {
	LoadStatus(ctx context.Context, id string) (*persistence.Status, error)
}

// WSConnHandler WwbSocketConnection wrapper
type WSConnHandler interface {
	HandleConnection(WsConn) error
	GetConnections(id string) ([]WsConn, bool)
}

// Data keeps data required for service work
type Data struct {
	Port      int
	DB        DB
	WSHandler WSConnHandler
}

// StartWebServer starts echo web service
func StartWebServer(data *Data) error {
	goapp.Log.Info().Msgf("Starting HTTP ROXY status service at %d", data.Port)
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

var promMdlw *prometheus.Prometheus

func init() {
	promMdlw = prometheus.NewPrometheus("roxy_status", nil)
}

func initRoutes(data *Data) *echo.Echo {
	e := echo.New()
	e.Use(middleware.Logger())
	promMdlw.Use(e)

	e.GET("/status/:id", statusHandler(data))
	e.GET("/live", live(data))
	e.GET("/subscribe", subscribeHandler(data))

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
	ID               string   `json:"id"`
	ErrorCode        string   `json:"errorCode,omitempty"`
	Error            string   `json:"error,omitempty"`
	Status           string   `json:"status"`
	RecognizedText   string   `json:"recognizedText,omitempty"`
	Progress         int32    `json:"progress,omitempty"`
	AudioReady       bool     `json:"audioReady,omitempty"`
	AvailableResults []string `json:"avResults,omitempty"`
}

func statusHandler(data *Data) func(echo.Context) error {
	return func(c echo.Context) error {
		defer goapp.Estimate("status method")()

		id := c.Param("id")
		if id == "" {
			return echo.NewHTTPError(http.StatusBadRequest, "No ID")
		}
		st, err := data.DB.LoadStatus(c.Request().Context(), id)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return echo.NewHTTPError(http.StatusInternalServerError, "Service error")
		}
		var res result
		if st == nil {
			res = result{ID: id, Status: "NOT_FOUND", Error: "NOT_FOUND", ErrorCode: "NOT_FOUND"}

		} else {
			res = *mapStatus(st)

		}
		return c.JSON(http.StatusOK, res)
	}
}

func mapStatus(st *persistence.Status) *result {
	return &result{ID: st.ID, Status: st.Status, Error: st.Error.String, ErrorCode: st.ErrorCode.String, Progress: st.Progress.Int32,
		AudioReady: st.AudioReady, AvailableResults: st.AvailableResults, RecognizedText: utils.FromSQLStr(st.RecognizedText)}
}

func validate(data *Data) error {
	if data.DB == nil {
		return fmt.Errorf("no DB")
	}
	if data.WSHandler == nil {
		return fmt.Errorf("no WSHandler")
	}
	return nil
}

var wsUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	}}

func subscribeHandler(data *Data) func(echo.Context) error {
	return func(c echo.Context) error {
		ws, err := wsUpgrader.Upgrade(c.Response(), c.Request(), nil)
		if err != nil {
			goapp.Log.Error().Err(err).Send()
			return err
		}
		defer ws.Close()

		return data.WSHandler.HandleConnection(ws)
	}
}
