package api

import (
	"context"
	"io"
)

// UploadData keeps structure for upload method
type UploadData struct {
	Params map[string]string
	Files  map[string]io.Reader
}

// StatusData keeps structure for status method
type StatusData struct {
	ID             string
	Text           string
	Completed      bool
	ErrorCode      string
	Error          string
	Status         string
	Progress       int
	AudioReady     bool
	AvResults      []string
	RecognizedText string `json:"recognizedText,omitempty"`
}

// FileData contains name and data
type FileData struct {
	Name    string
	Content []byte
}

// Transcriber provides transcription
type Transcriber interface {
	Upload(ctx context.Context, audioFunc func(context.Context) (*UploadData, func(), error)) (string, error)
	HookToStatus(ctx context.Context, ID string) (<-chan StatusData, func(), error)
	GetStatus(ctx context.Context, ID string) (*StatusData, error)
	GetAudio(ctx context.Context, ID string) (*FileData, error)
	GetResult(ctx context.Context, ID, name string) (*FileData, error)
	Clean(ctx context.Context, ID string) error
}
