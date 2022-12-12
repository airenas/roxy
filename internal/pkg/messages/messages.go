package messages

import (
	amessages "github.com/airenas/async-api/pkg/messages"
)

const (
	st = "ROXY/"
	// Upload queue name
	Upload = st + "Work:wrk-upload"
	// Work queue name
	Work = st + "Work"
	// Fail queue name
	Fail = st + "Work:wrk-fail"
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
	ExternalID string `json:"extID,omitempty"`
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
}
