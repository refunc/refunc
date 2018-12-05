package helloworld

import (
	"github.com/refunc/refunc/pkg/builtins"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/version"
)

func helloWorld(request *messages.InvokeRequest) (result interface{}, err error) {
	return struct {
		Msg string `json:"message"`
	}{
		Msg: "Hello world!",
	}, nil
}

func clusterInfo(request *messages.InvokeRequest) (result interface{}, err error) {
	return struct {
		RefuncVersion  string `json:"refuncVersion"`
		AgentVersion   string `json:"agentVersion,omitempty"`
		LoaderVersion  string `json:"loaderVersion,omitempty"`
		SidecarVersion string `json:"sidcarVersion,omitempty"`
	}{
		version.Version,
		version.AgentVersion,
		version.LoaderVersion,
		version.SidecarVersion,
	}, nil
}

func init() {
	builtins.RegisterBuiltin("helloworld", "", helloWorld)
	builtins.RegisterBuiltin("cluster-info", "", clusterInfo)
}
