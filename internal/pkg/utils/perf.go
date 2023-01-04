package utils

import (
	"net/http"
	"strconv"

	"github.com/airenas/go-app/pkg/goapp"

	_ "net/http/pprof"
)

func RunPerfEndpoint() {
	port := goapp.Config.GetInt("debug.port")
	if port > 0 {
		goapp.Log.Info().Msgf("Starting Debug http endpoint at [::]:%d", port)
		portStr := strconv.Itoa(port)
		err := http.ListenAndServe(":"+portStr, nil)
		if err != nil {
			goapp.Log.Error().Err(err).Msg("can't start Debug endpoint")
		}
	} else {
		goapp.Log.Info().Msgf("no debug.port provided - skip perf?")
	}
}
