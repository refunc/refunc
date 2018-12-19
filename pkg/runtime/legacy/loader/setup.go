package loader

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/refunc/refunc/pkg/env"
)

func (ld *Agent) prepare(fnrt *FuncRuntime) func() (*exec.Cmd, error) {
	// FIXME: when rpc v2 finished
	os.Unsetenv("REFUNC_APP")

	// prepare locals
	for k, v := range fnrt.Spec.Runtime.Envs {
		if v != "" {
			// try to expand env
			if strings.HasPrefix(v, "$") {
				v = os.ExpandEnv(v)
			}
			os.Setenv(k, v)
		}
	}

	// insert refunc sepcific environ
	os.Setenv("REFUNC_ENV", "cluster")
	os.Setenv("REFUNC_NAMESPACE", fnrt.Namespace)
	os.Setenv("REFUNC_NAME", fnrt.Name)
	os.Setenv("REFUNC_HASH", fnrt.Spec.Hash)

	if fnrt.Spec.Runtime.Credentials.Token != "" {
		os.Setenv("REFUNC_TOKEN", fnrt.Spec.Runtime.Credentials.Token)
	}

	os.Setenv("REFUNC_ACCESS_KEY", fnrt.Spec.Runtime.Credentials.AccessKey)
	os.Setenv("REFUNC_SECRET_KEY", fnrt.Spec.Runtime.Credentials.SecretKey)

	os.Setenv("REFUNC_MINIO_SCOPE", fnrt.Spec.Runtime.Permissions.Scope)
	os.Setenv("REFUNC_MAX_TIMEOUT", fmt.Sprintf("%d", fnrt.Spec.Runtime.Timeout))

	// TODO(bin): remove some day
	os.Setenv("REFUNC_DEBUG", "true")

	// reload envs
	env.RefreshEnvs()

	var env []string
	for _, kv := range os.Environ() {
		env = append(env, kv)
	}

	loader, command := fnrt.Spec.Cmd[0], fnrt.Spec.Cmd[1:]
	// change to func root
	os.Chdir(ld.funcDir())

	return func() (*exec.Cmd, error) {
		cmd := exec.CommandContext(ld.ctx, loader, command...)
		cmd.Env = env
		return cmd, nil
	}
}
