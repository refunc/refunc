package client

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/refunc/refunc/pkg/messages"
)

// TaskParser parses action from task
type TaskParser interface {
	// SetResult set result or error, and terminates task resolver
	SetResult(data []byte, err error)
	// UpdateLog updates log source
	UpdateLog(line []byte)
}

// ParseAction parses action for a task parser
func ParseAction(raw []byte, p TaskParser) (next bool) {
	var action *messages.Action
	err := json.Unmarshal(raw, &action)
	if err != nil {
		if len(raw) < 256 && isPrintable(string(raw)) {
			p.SetResult(nil, fmt.Errorf("task: %s", raw))
		} else {
			p.SetResult(nil, fmt.Errorf("task: invalid response, %v", err))
		}
		return
	}

	switch action.Type {
	case messages.Error:
		var errmsg messages.ErrorMessage
		err := json.Unmarshal(action.Payload, &errmsg)
		if err != nil {
			p.SetResult(nil, fmt.Errorf("task: json error, %v", unquote(action.Payload)))
			return
		}
		p.SetResult(nil, errmsg)
		return

	case messages.Response:
		var rsp messages.InvokeResponse
		//lambda function return value always is json-formated.
		//https://docs.aws.amazon.com/lambda/latest/dg/python-handler.html#python-handler-return
		//https://docs.aws.amazon.com/lambda/latest/dg/nodejs-handler.html
		err := json.Unmarshal(action.Payload, &rsp)
		if err != nil {
			p.SetResult(nil, fmt.Errorf("task: json error, %v", unquote(action.Payload)))
			return
		}
		if rsp.Error != nil {
			p.SetResult(nil, rsp.Error)
			return
		}
		p.SetResult(rsp.Payload, nil)
		return

	case messages.Log:
		p.UpdateLog(action.Payload)

	default:
		p.SetResult(nil, fmt.Errorf("unsupported action type: %q", action.Type))
		return
	}

	return true
}

// SetReqeustDeadline parse and set deadline for request
func SetReqeustDeadline(ctx context.Context, request *messages.InvokeRequest) {
	deadline := request.Deadline
	if deadline.IsZero() {
		deadline = time.Now().Add(messages.DefaultJobTimeout)
	}
	timeout := GetTimeoutHint(ctx)
	if timeout > 500*time.Millisecond && time.Now().Add(timeout).Before(deadline) {
		deadline = time.Now().Add(timeout)
	}
	request.Deadline = deadline
}
