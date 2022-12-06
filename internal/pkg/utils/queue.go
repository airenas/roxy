package utils

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/airenas/go-app/pkg/goapp"
	"github.com/vgarvardt/gue/v5"
)

// CreateHandler helper func to wrapp gue worker main func
func CreateHandler[TM any, SD any](data *SD, hf func(context.Context, *TM, *SD) error) gue.WorkFunc {
	return func(ctx context.Context, j *gue.Job) error {
		var m TM
		if err := json.Unmarshal(j.Args, &m); err != nil {
			return fmt.Errorf("could not unmarshal message: %w", err)
		}
		goapp.Log.Info().Str("queue", j.Queue).Str("type", j.Type).Int32("errCount", j.ErrorCount).Msg("got msg")
		if j.ErrorCount > 2 {
			goapp.Log.Error().Int32("time", j.ErrorCount).Str("lastError", j.LastError.String).Msg("msg failed, will not retry")
			return nil
		}
		return hf(ctx, &m, data)
	}
}
