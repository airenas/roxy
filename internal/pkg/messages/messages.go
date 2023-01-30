package messages

import (
	"time"

	amessages "github.com/airenas/async-api/pkg/messages"
)

const (
	st = "ROXY/"
	// Upload queue name
	Upload = st + "Work:wrk-upload"
	// Work queue name
	Work = st + "Work"
	// Inform  queue name
	Inform = st + "Inform"
	// StatusChange queue name
	StatusChange = st + "StatusChange"
)

// ASRMessage main message passing through in roxy asr system
type ASRMessage struct {
	amessages.QueueMessage
	RequestID string `json:"requestID,omitempty"`
}

// CleanMessage message to clean external data
type CleanMessage struct {
	amessages.QueueMessage
	ExternalID  string `json:"extID,omitempty"`
	Transcriber string `json:"transcriber,omitempty"`
}

// NewMessageFrom creates a copy of a message
func NewMessageFrom(m *ASRMessage) *ASRMessage {
	return &ASRMessage{QueueMessage: m.QueueMessage, RequestID: m.RequestID}
}

// StatusMessage main message passing through in roxy asr system
type StatusMessage struct {
	amessages.QueueMessage
	Status           string   `json:"status,omitempty"`
	Error            string   `json:"error,omitempty"`
	Progress         int      `json:"progress,omitempty"`
	ErrorCode        string   `json:"errorCode,omitempty"`
	AudioReady       bool     `json:"audioReady,omitempty"`
	AvailableResults []string `json:"avResults,omitempty"`
	ExternalID       string   `json:"extID,omitempty"`
	RecognizedText   string   `json:"recognizedText,omitempty"`

	Transcriber string `json:"transcriber,omitempty"`
}

// ASRMessage main message passing through in roxy asr system
type Options struct {
	Queue string
	After time.Duration
}

func DefaultOpts(queue string) *Options {
	return &Options{Queue: queue}
}

func (opt *Options) Delay(after time.Duration) *Options {
	opt.After = after
	return opt
}
