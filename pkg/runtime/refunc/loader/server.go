package loader

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"k8s.io/klog"

	"github.com/gliderlabs/ssh"
	shellwords "github.com/mattn/go-shellwords"
	nats "github.com/nats-io/go-nats"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/utils/rtutil"
)

// Agent loads and hosts the entry
type Agent struct {
	WorkDir string

	Loader string

	// ssh admin access
	AccessKey  string
	HostKeyPEM []byte

	listener net.Listener

	ssh struct {
		srv         *ssh.Server
		authHandler atomic.Value
	}

	initOnce sync.Once

	ctx    context.Context
	cancel context.CancelFunc

	setupOnce struct {
		sync.Mutex
		done bool
		sync.Once
		err error
	}
}

type srvInterface interface {
	Serve() error
}

// Server listens at addr and serves forever
func (ld *Agent) Server(addr string) error {
	if addr == "" {
		addr = ":http"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()

	var srv func() error
	ld.initOnce.Do(func() {
		srv, err = ld.getServer(ln)
	})
	if err != nil {

		return err
	}
	if srv == nil {
		return errors.New("agent: already start once")
	}
	// block and serve forever
	return checkListenerErr(srv())
}

// Shutdown gracefully shuts down the server without interrupting any active connections.
func (ld *Agent) Shutdown(ctx context.Context) error {
	if ld.listener != nil {
		ld.cancel()
		return checkListenerErr(ld.listener.Close())
	}

	return nil
}

func (ld *Agent) getServer(ln net.Listener) (func() error, error) {

	// try reload
	fnrt, err := ld.loadFunc()
	if fnrt != nil {
		// close
		ln.Close()
		return func() error { return ld.setupRefunc(fnrt, true) }, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}

	ld.listener = tcpKeepAliveListener{ln.(*net.TCPListener)}

	ld.ctx, ld.cancel = context.WithCancel(context.Background())

	klog.Infof("(agent) starting agent in %s listen to %v with loader %s", ld.funcDir(), ln.Addr(), filepath.Base(ld.Loader))

	ld.ssh.srv = &ssh.Server{
		Handler: ld.setupHandler(),
		PasswordHandler: func(ctx ssh.Context, password string) bool {
			val := ld.ssh.authHandler.Load()
			if h, ok := val.(ssh.PasswordHandler); ok {
				return h(ctx, password)
			}
			if rtutil.ComparePassword(ld.AccessKey, password) {
				for _, c := range adminCmds {
					if ctx.User() == c {
						return true
					}
				}
			}
			return false
		},
	}
	if ld.HostKeyPEM == nil {
		ld.HostKeyPEM = defaultHostKey
	}
	ld.ssh.srv.SetOption(ssh.HostKeyPEM(ld.HostKeyPEM))

	return func() error { return ld.ssh.srv.Serve(ld.listener) }, nil
}

func (ld *Agent) updateHandler() error {
	refunc, err := ld.loadFunc()
	if err != nil {
		return err
	}
	return ld.setupRefunc(refunc, false)
}

// using variable for testing
var refuncFileName = "refunc.json"

// the place under the working dir in which to execute scripts
const execDir = "root"

func (ld *Agent) funcDir() string {
	return filepath.Join(ld.WorkDir, execDir)
}

func (ld *Agent) loadFunc() (*FuncRuntime, error) {
	jsonpath := filepath.Join(ld.WorkDir, refuncFileName)
	if _, err := os.Stat(jsonpath); err != nil {
		return nil, err
	}

	bts, err := ioutil.ReadFile(jsonpath)
	if err != nil {
		return nil, err
	}

	var refunc *FuncRuntime
	err = json.Unmarshal(bts, &refunc)
	if err != nil {
		return nil, err
	}

	if refunc.Spec.Entry == "" && ld.Loader == "" {
		return nil, errors.New("agent: no entry found in refunc and agent")
	}

	args, err := shellwords.Parse(refunc.Spec.Entry)
	if err != nil {
		return nil, err
	}

	loader := ld.Loader
	if loader == "" {
		loader = args[0]
		args = args[1:]
	}

	// insert current working dir into PATH
	os.Setenv("PATH", strings.Join(append(
		filepath.SplitList(os.Getenv("PATH")),
		ld.WorkDir,
		ld.funcDir(),
		func() string {
			addindir := os.Getenv("REFUNC_ADDIN_DIR")
			if addindir == "" {
				return "/var/lib/refunc/addins"
			}
			return addindir
		}(),
	), ":"))
	loader, err = exec.LookPath(loader)
	if err != nil {
		return nil, err
	}
	ld.Loader = loader // set correct loader

	// override the entry
	refunc.Spec.Cmd = append([]string{loader}, args...)

	return refunc, nil
}

func (ld *Agent) setupRefunc(fnrt *FuncRuntime, sync bool) error {
	ld.setupOnce.Lock()
	defer ld.setupOnce.Unlock()
	if ld.setupOnce.done {
		return nil
	}

	klog.Infof("(agent) setting up refunc: %s", fnrt.Name)
	// kickoff message bumper
	opener := ld.prepare(fnrt)
	// connect to nats
	conn, err := env.NewNatsConn(nats.Name(fnrt.Namespace + "/" + fnrt.Name))
	if err != nil {
		return fmt.Errorf("failed to connect to nats %s, %v", env.GlobalNATSEndpoint, err)
	}
	// disable ssh handler
	ld.updateSSHPasswdHandler(func(ctx ssh.Context, pass string) bool {
		return false
	})
	ld.setupOnce.done = true

	if !sync {
		go func() {
			defer conn.Close()
			ld.serve(fnrt, opener, conn)
		}()
	} else {
		defer conn.Close()
		ld.serve(fnrt, opener, conn)
	}
	return nil
}
