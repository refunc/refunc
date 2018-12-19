package loader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"k8s.io/klog"

	"github.com/gliderlabs/ssh"
	"github.com/refunc/refunc/pkg/utils"
)

var adminCmds = []string{"setup"}

func (ld *Agent) setupHandler() ssh.Handler {

	os.Chdir(ld.WorkDir)

	return func(session ssh.Session) {
		// current only load support
		cmd := session.User()
		_, _, pty := session.Pty()
		klog.Infof("(agent) accept [%s] from %v, pty: %v", cmd, session.RemoteAddr(), pty)
		if pty && checkError(session, errors.New("Pty is forbidden")) {
			return
		}

		t0 := time.Now()
		defer func() {
			klog.Infof("(agent) taking %v to initialize", time.Since(t0))
		}()

		// prepare a temp working dir
		dir, err := ioutil.TempDir("", "ra")
		if checkError(session, err) {
			return
		}
		defer os.RemoveAll(dir)

		fp := filepath.Join(dir, "loader")
		file, err := os.OpenFile(fp, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0700)
		if checkError(session, err) {
			return
		}

		wn, err := io.Copy(file, session)
		if checkError(session, err) {
			file.Close()
			return
		}
		klog.Infof("(agent) %s wrote to %s", utils.ByteSize(uint64(wn)), fp)
		file.Close()

		// max timeout for setting up a func
		ctx, cancel := context.WithTimeout(session.Context(), 3*time.Second)
		defer cancel()

		p := exec.CommandContext(ctx, fp)
		p.Env = append(
			[]string{
				"REFUNC_DIR=" + ld.WorkDir,
				"REFUNC_ROOT_DIR=" + ld.funcDir(),
			},
			append(session.Environ(), os.Environ()...)...,
		)

		go ld.forwardSignals(ctx, session, p)

		p.Stdout = session
		p.Stderr = session.Stderr()

		if err := p.Run(); checkError(session, err) {
			klog.Errorf("(agent) loader exited with error, %v", err)
			return
		}
		if err := ld.updateHandler(); checkError(session, err) {
			klog.Errorf("(agent) faild to update handler, %v", err)
		}
	}
}

var signalMap = map[ssh.Signal]os.Signal{
	ssh.SIGABRT: syscall.SIGABRT,
	ssh.SIGALRM: syscall.SIGALRM,
	ssh.SIGFPE:  syscall.SIGFPE,
	ssh.SIGHUP:  syscall.SIGHUP,
	ssh.SIGILL:  syscall.SIGILL,
	ssh.SIGINT:  syscall.SIGINT,
	ssh.SIGKILL: syscall.SIGKILL,
	ssh.SIGPIPE: syscall.SIGPIPE,
	ssh.SIGQUIT: syscall.SIGQUIT,
	ssh.SIGSEGV: syscall.SIGSEGV,
	ssh.SIGTERM: syscall.SIGTERM,
	ssh.SIGUSR1: syscall.SIGUSR1,
	ssh.SIGUSR2: syscall.SIGUSR2,
}

func (ld *Agent) forwardSignals(ctx context.Context, s ssh.Session, p *exec.Cmd) {
	sigC := make(chan ssh.Signal, 2)
	s.Signals(sigC)
	defer s.Signals(nil)

	for {
		var signal ssh.Signal
		select {
		case <-ctx.Done():
			return
		case signal = <-sigC:
		}

		if p.ProcessState == nil || !p.ProcessState.Exited() {
			return
		}

		if ossig, ok := signalMap[signal]; ok {
			p.Process.Signal(ossig)
			continue
		}
		klog.Warningf("(agent) cannot forward signal %q", signal)
		return
	}
}

func checkError(s interface {
	Stderr() io.ReadWriter
	Exit(code int) error
}, err error) bool {
	if err != nil {
		code := 255

		// http://stackoverflow.com/questions/10385551/get-exit-code-go
		if exiterr, ok := err.(*exec.ExitError); ok {
			// The program has exited with an exit code != 0

			// This works on both Unix and Windows. Although package
			// syscall is generally platform dependent, WaitStatus is
			// defined for both Unix and Windows and in both cases has
			// an ExitStatus() method with the same signature.
			if status, ok := exiterr.Sys().(syscall.WaitStatus); ok {
				code = status.ExitStatus()
			}
		}

		fmt.Fprintf(s.Stderr(), "{\"code\":%d,\"msg\":%q}\r\n", code, err.Error())
		s.Exit(code)
		return true
	}
	return false
}

var defaultHostKey = []byte(`
-----BEGIN RSA PRIVATE KEY-----
MIICXgIBAAKBgQDN1vszD0qcz1JFbCRVQ4mqy11CzqkeCm1lo8jD/bLUQF9nlHaV
kgDuOJByiGLWbNt5MjOs77+H1dtk0llCa+f1y4iohUEcUjt74aoMUlAV9wV6YBAK
nEyGMldz89QDi7fm0u+rGpxXcwwJ6O/WNml9wJhQPrxtlHLRoVy9XE15KwIDAQAB
AoGBALKYqEYKK4PZQpnnlbLBMc6WOun/Y68j/v1kWYrsMeCFpgG6SBXIo7QOMg6e
FZvUwazriPiw4G8ceAqHlFjURWJpfC/pEIP9fwNlthXCKYEf2E0FVeyc3bklMSAL
eJpI+IAvwu+rRiOyA1BDiMc74zRMyaHAiGxOp8NOhzM9WKnJAkEA6RvNn9OLf/rT
tHl1hh2cc9oBFI1OZVNNGcJYeRRUY0WRp/nK1cvfvwzARNmXNpObz62LaDQaicYm
VYKx582jxQJBAOINqE9fbvvdb8/mksDyA1Ym5C4DdUEfx014p0B9RWXVQEEumG29
IsxcvWzjGEiClJB87OxrslKcxu22LGhZSC8CQQDfrs0+S3k2AlM5f781RZ7GUG/u
77VFZ4y5ZhMNhGOBqtUc8YYgZ3S5WBv7NSxzs2q0+tulzzGT+O756OKcA2jdAkEA
0SbrzIyzJkxq8MQYkncZiTOwubYvXhMmF1MEBNIjTKYzrluLYzW1JbrE9SNlS2mu
RcWgfNrkgjVWhYihq+a3twJAA6SYPnSiz9aMqh/UhydTAriRRlG1PspoP92gH9f5
JN/uC3r7mtxjfOkchKUzC85C9kZaKbWBhKSb9R4T2UGKPA==
-----END RSA PRIVATE KEY-----
`)

func (ld *Agent) updateSSHPasswdHandler(ah ssh.PasswordHandler) {
	ld.ssh.authHandler.Store(ah)
}
