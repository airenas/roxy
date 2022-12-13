package api

import "io"

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
