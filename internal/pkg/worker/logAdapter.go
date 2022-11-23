package worker

import (
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/rs/zerolog"
	"github.com/vgarvardt/gue/v5/adapter"
)

type logAdapter struct {
	fields []adapter.Field
}

func newGueLoggerAdapter() *logAdapter {
	return &logAdapter{}
}

// Debug implements adapter.Logger
func (l *logAdapter) Debug(msg string, fields ...adapter.Field) {
	l.do(goapp.Log.Debug(), fields...).Msg(msg)
}

// Info implements adapter.Logger
func (l *logAdapter) Info(msg string, fields ...adapter.Field) {
	l.do(goapp.Log.Info(), fields...).Msg(msg)
}

// Error implements adapter.Logger
func (l *logAdapter) Error(msg string, fields ...adapter.Field) {
	l.do(goapp.Log.Error(), fields...).Msg(msg)
}

// With implements adapter.Logger
func (l *logAdapter) With(fields ...adapter.Field) adapter.Logger {
	return &logAdapter{fields: fields}
}

func (l *logAdapter) do(le *zerolog.Event, fields ...adapter.Field) *zerolog.Event {
	for _, f := range l.fields {
		le = le.Interface(f.Key, f.Value)
	}
	for _, f := range fields {
		le = le.Interface(f.Key, f.Value)
	}
	return le
}
