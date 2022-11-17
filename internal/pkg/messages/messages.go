package messages

import (
	amessages "github.com/airenas/async-api/pkg/messages"
)

const (
	st = "ROXY/"
	// Upload queue name
	Upload = st + "Upload"
	// WORK queue name
	Work = st + "Work"
	// Fail queue name
	Fail = st + "Fail"
	// Inform  queue name
	Inform = st + "Inform"
)

// ASRMessage main message passing through in roxy asr system
type ASRMessage struct {
	amessages.QueueMessage
	RequestID string `json:"requestID,omitempty"`
}

// NewMessageFrom creates a copy of a message
func NewMessageFrom(m *ASRMessage) *ASRMessage {
	return &ASRMessage{QueueMessage: m.QueueMessage, RequestID: m.RequestID}
}
