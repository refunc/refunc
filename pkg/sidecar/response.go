package sidecar

import (
	"encoding/json"
	"net/http"

	"github.com/refunc/refunc/pkg/messages"
)

func writeError(w http.ResponseWriter, code int, err error) {
	switch v := err.(type) {
	case messages.ErrorMessage:
		writeErrorResponse(w, code, v.Type, v.Message)
	default:
		writeErrorResponse(w, code, getErrorType(err), err.Error())
	}
}

func writeErrorResponse(w http.ResponseWriter, code int, errType, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	//nolint:errcheck
	w.Write(func() []byte {
		bts, _ := json.Marshal(struct {
			ErrorType    string `json:"errorType"`
			ErrorMessage string `json:"errorMessage"`
		}{
			ErrorType:    errType,
			ErrorMessage: errMsg,
		})
		return bts
	}())
}

func writeStatus(w http.ResponseWriter, code int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	//nolint:errcheck
	w.Write(func() []byte {
		bts, _ := json.Marshal(struct {
			Status string `json:"status"`
		}{
			Status: status,
		})
		return bts
	}())
}
