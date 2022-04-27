package messages

import (
	"encoding/json"
	"fmt"
	"reflect"
	"time"
)

const (
	//https://docs.aws.amazon.com/lambda/latest/dg/gettingstarted-limits.html#function-configuration-deployment-and-execution

	//MaxPayloadSize is hard coded max size of each event can carray
	MaxPayloadSize = 6 << 20 // 6 MB
	// MaxTimeout is max execution time enfoced by runtime
	MaxTimeout = 4 * time.Hour
	// DefaultJobTimeout is hard coded max timeout for a task, same as AWS
	DefaultJobTimeout = 9 * time.Minute
)

// MessageType alias type for event's type
type MessageType string

// Predefined actions
const (
	// The payload is a request object
	Request MessageType = "req"
	// The payload is a response object
	Response MessageType = "rsp"
	// The payload is a emitted message
	Emit MessageType = "emit"
	// The payload is a logging line
	Log MessageType = "log"
	// Indicates the communication end with a error
	Error MessageType = "err"
	// Ping action
	Ping MessageType = "_ping"
)

// Action is basic unit between loader and a func
// {"a":"req","p":"hello world"}\r\n
type Action struct {
	Type    MessageType     `json:"a"`
	Payload json.RawMessage `json:"p,omitempty"`
}

// InvokeRequest wraps data pass to function invocation
type InvokeRequest struct {
	Args      json.RawMessage        `json:"args"`
	RequestID string                 `json:"rid,omitempty"`
	TraceID   string                 `json:"tid,omitempty"`
	Deadline  time.Time              `json:"deadline,omitempty"`
	User      string                 `json:"user,omitempty"`
	Options   map[string]interface{} `json:"options,omitempty"`
}

// InvokeResponse is the message returns from a function
type InvokeResponse struct {
	Payload json.RawMessage `json:"payload,omitempty"`
	Error   *ErrorMessage   `json:"error,omitempty"`
	// Content type of payload
	ContentType string `json:"ContentType,omitempty"`
}

// ErrorMessage wraps error information during a invocation
type ErrorMessage struct {
	Message    string        `json:"errorMessage"`
	Type       string        `json:"errorType,omitempty"`
	StackTrace []interface{} `json:"stackTrace,omitempty"`
	Fatal      bool          `json:"fatal"`
}

// StackFrame is frame information of a exception
type StackFrame struct {
	Path  string      `json:"path"`
	Line  json.Number `json:"line"`
	Label string      `json:"label,omitempty"`
}

var (
	// PingMsg bytes of ping action message
	PingMsg = []byte("{\"a\":\"_ping\"}\r\n")

	// TokenCRLF is token of \r\n
	TokenCRLF = []byte{'\r', '\n'}
)

// Error implements error interface
func (em ErrorMessage) Error() string {
	if em.Message == "" {
		if em.Type != "" {
			return em.Type
		}
		return "<nil>"
	}
	if em.Type != "" {
		return fmt.Sprintf("%s: %s", em.Type, em.Message)
	}
	return em.Message
}

// MustFromObject creates new from a object
func MustFromObject(obj interface{}) json.RawMessage {
	bts, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return json.RawMessage(bts)
}

// GetErrorMessage creates ErrorMessage from given golang error
func GetErrorMessage(err error) *ErrorMessage {
	if err == nil {
		return nil
	}

	var errMsg ErrorMessage
	switch v := err.(type) {
	case ErrorMessage:
		errMsg = v
	default:
		errMsg.Type = getErrorType(err)
		errMsg.Message = err.Error()
	}
	return &errMsg
}

// GetErrActionBytes create action bytes of a error
func GetErrActionBytes(err error) (bts []byte) {
	if err == nil {
		return nil
	}

	bts, _ = json.Marshal(&Action{
		Type:    Error,
		Payload: MustFromObject(GetErrorMessage(err)),
	})
	return
}

func getErrorType(err interface{}) string {
	errorType := reflect.TypeOf(err)
	if errorType.Kind() == reflect.Ptr {
		return errorType.Elem().Name()
	}
	return errorType.Name()
}
