package utils

import (
	"github.com/airenas/go-app/pkg/goapp"
	"github.com/rs/zerolog"
	"github.com/vgarvardt/gue/v5/adapter"
)

type GueLogAdapter struct {
	fields []adapter.Field
}

func NewGueLoggerAdapter() *GueLogAdapter {
	return &GueLogAdapter{}
}

// Debug implements adapter.Logger
func (l *GueLogAdapter) Debug(msg string, fields ...adapter.Field) {
	l.do(goapp.Log.Debug(), fields...).Msg(msg)
}

// Info implements adapter.Logger
func (l *GueLogAdapter) Info(msg string, fields ...adapter.Field) {
	l.do(goapp.Log.Info(), fields...).Msg(msg)
}

// Error implements adapter.Logger
func (l *GueLogAdapter) Error(msg string, fields ...adapter.Field) {
	l.do(goapp.Log.Error(), fields...).Str(zerolog.ErrorFieldName, msg).Send()
}

// With implements adapter.Logger
func (l *GueLogAdapter) With(fields ...adapter.Field) adapter.Logger {
	return &GueLogAdapter{fields: fields}
}

func (l *GueLogAdapter) do(le *zerolog.Event, fields ...adapter.Field) *zerolog.Event {
	for _, f := range l.fields {
		le = le.Interface(f.Key, f.Value)
	}
	for _, f := range fields {
		le = le.Interface(f.Key, f.Value)
	}
	return le
}
