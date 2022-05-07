package loader

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"k8s.io/klog"

	"github.com/Arvintian/websocketrwc"
	shellwords "github.com/mattn/go-shellwords"
	"github.com/mholt/archiver"
	"github.com/nats-io/nuid"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/runtime/types"
	"github.com/refunc/refunc/pkg/utils"
)

func (ld *simpleLoader) loadFunc() (*types.Function, error) {
	jsonpath := filepath.Join(RefuncRoot, ConfigFile)
	if _, err := os.Stat(jsonpath); err != nil {
		return nil, err
	}

	bts, err := ioutil.ReadFile(jsonpath)
	if err != nil {
		return nil, err
	}

	var fn *types.Function
	err = json.Unmarshal(bts, &fn)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(filepath.Join(RefuncRoot, ".setup")); err != nil {
		if err := ld.setup(fn); err != nil {
			return nil, err
		}
	}

	return fn, nil
}

func (ld *simpleLoader) setup(fn *types.Function) (err error) {
	// nolint:errcheck
	withTmpFloder(func(folder string) {
		// download
		var filename string
		switch {
		case strings.HasPrefix(fn.Spec.Body, "http"): // http or https
			filename, err = saveURL(fn.Spec.Body, folder)
		case strings.HasPrefix(fn.Spec.Body, "base64://"):
			filename, err = saveBase64(fn.Spec.Body, folder)
		default:
			err = fmt.Errorf(`loader: Only support "http(s)//" or "base64//, got "%s"`, func() string {
				if len(fn.Spec.Body) > 10 {
					return fn.Spec.Body[:9]
				}
				return fn.Spec.Body
			}())
		}
		if err != nil {
			return
		}

		// unpack source code
		klog.Infof("(loader) unpacking %s to %s", filepath.Base(filename), ld.taskRoot())
		err = archiver.Unarchive(filename, ld.taskRoot())
		if err == nil && os.Geteuid() == 0 {
			klog.Info("(loader) fix task folder's permission chown slicer:497")
			// nolint:errcheck
			filepath.Walk(ld.taskRoot(), func(path string, f os.FileInfo, err error) error {
				klog.V(4).Infof("(loader) chown for %q", path)
				os.Chown(path, 498, 497)
				return nil
			})
		}
	})

	if err != nil {
		return
	}

	cfgPath := filepath.Join(RefuncRoot, ConfigFile)
	klog.Infof("(loader) setup for %s is done, write %s", fn.Name, cfgPath)
	err = ioutil.WriteFile(cfgPath, messages.MustFromObject(fn), 0755)

	if file, ferr := os.OpenFile(filepath.Join(RefuncRoot, ".setup"), os.O_RDWR|os.O_CREATE, 0755); ferr == nil {
		// touch done
		file.Close()
	}

	return
}

func (ld *simpleLoader) prepare(fn *types.Function) (*exec.Cmd, error) {
	wid := nuid.Next()
	apiAddr := fn.Spec.Runtime.Envs["AWS_LAMBDA_RUNTIME_API"]

	// redirect func's stdout/stderr log
	var stdout io.Writer = os.Stderr
	if apiAddr != "" {
		if conn, _, err := websocketrwc.Dial(fmt.Sprintf("ws://%s/2018-06-01/%s/log", apiAddr, wid), nil, nil); err != nil {
			klog.Errorf("(loader) redirect stdout/stderr log faild %v", err)
		} else {
			klog.Infof("(loader) redirect stdout/stderr log to %s.%s", fn.Spec.Runtime.Envs["REFUNC_LOG_ENDPOINT"], wid)
			stdout = conn
			// read loop for driver ping/pong handler
			go func() {
				var buf [128]byte
				for {
					if _, err := conn.Read(buf[:]); err != nil {
						klog.Errorf("(loader) redirect stdout/stderr stream reader faild %v", err)
						return
					}
				}
			}()
		}
	}

	// proxy runtime api
	if apiAddr != "" {
		if runtimeAddr, err := withProxyRuntimeAPI(wid, apiAddr); err != nil {
			klog.Errorf("(loader) prepare proxy runtime error %v", err)
		} else {
			klog.Infof("(loader) proxy worker %s runtime at %s", wid, runtimeAddr)
			apiAddr = runtimeAddr
		}
	}

	// prepare locals
	var env []string
	env = append(env, os.Environ()...)
	for k, v := range fn.Spec.Runtime.Envs {
		if k == "AWS_LAMBDA_RUNTIME_API" {
			v = apiAddr
		}
		if v != "" {
			// try to expand env
			if strings.HasPrefix(v, "$") {
				v = os.ExpandEnv(v)
			}
			env = append(env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	args, err := shellwords.Parse(ld.mainExe())
	if err != nil {
		return nil, err
	}

	if len(args) == 0 {
		args = []string{DefaultMain}
	}

	cmdPath := args[0]

	if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
		cmdPath = DefaultMain
		if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
			cmdPath = AlterMainPath
			if _, err := os.Stat(cmdPath); os.IsNotExist(err) {
				return nil, fmt.Errorf("loader: couldn't find valid bootstrap(s): [/var/task/bootstrap /opt/bootstrap]")
			}
		}
	}

	// override the entry
	fn.Spec.Cmd = append([]string{cmdPath}, args[1:]...)

	cmd := exec.CommandContext(ld.ctx, fn.Spec.Cmd[0], fn.Spec.Cmd[1:]...)
	cmd.Env = env
	cmd.Dir = ld.taskRoot()
	cmd.Stdout = stdout
	cmd.Stderr = stdout
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if os.Geteuid() == 0 {
		klog.Info("(loader) will start using user sbx_user1051")
		cmd.SysProcAttr.Credential = &syscall.Credential{Uid: 496, Gid: 495}
	}
	return cmd, nil
}

func saveURL(u string, folder string) (filename string, err error) {
	parsedURL, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	rsp, err := http.Get(parsedURL.String())
	if err != nil {
		return "", err
	}
	defer rsp.Body.Close()

	if rsp.StatusCode >= 300 {
		return "", fmt.Errorf("loader: unable to download file, got %v", rsp.StatusCode)
	}

	filename = filepath.Join(folder, path.Base(parsedURL.Path))
	file, err := os.OpenFile(filename, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return "", err
	}

	n, err := io.Copy(file, rsp.Body)
	if err != nil {
		return "", err
	}
	klog.Infof("(loader) downloader write %s to %s", utils.ByteSize(uint64(n)), filename)

	return filename, nil
}

func saveBase64(u string, folder string) (filename string, err error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return "", err
	}
	encoded := strings.TrimPrefix(parsed.Path, "/")
	bts, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		klog.Warningf("(loader) decode base64, %v, try decode url", err)
		bts, err = base64.URLEncoding.DecodeString(encoded)
	}
	if err != nil {
		return "", err
	}
	filename = filepath.Join(folder, path.Base(parsed.Host))
	err = ioutil.WriteFile(filename, bts, 0755)
	if err != nil {
		return "", err
	}
	klog.Infof("(loader) base64 write %s to %s", utils.ByteSize(uint64(len(bts))), filename)

	return
}

func withTmpFloder(fn func(dir string)) error {
	folder, err := ioutil.TempDir("", "unpack")
	if err != nil {
		return err
	}
	defer os.RemoveAll(folder)

	fn(folder)
	return nil
}

func withConcurrency(fn *types.Function) int {
	if fn.Annotations == nil {
		return 1
	}
	if s, ok := fn.Annotations[AnnotationLambdaConcurrency]; ok {
		v, err := strconv.Atoi(s)
		if err != nil {
			klog.Errorf("lambda concurrency setting error %v", err)
			return 1
		}
		if v < 1 {
			return 1
		}
		if v > MaxLambdaConcurrency {
			klog.Errorf("lambda concurrency setting error, %d > max %d, will apply with max.", v, MaxLambdaConcurrency)
			return MaxLambdaConcurrency
		}
		return v
	}
	return 1
}

func withProxyRuntimeAPI(wid string, apiAddr string) (string, error) {
	url, err := url.Parse(fmt.Sprintf("http://%s/", apiAddr))
	if err != nil {
		return "", err
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}

	go func(l net.Listener) {
		defer l.Close()
		server := &http.Server{}
		handler := &http.ServeMux{}
		proxy := httputil.NewSingleHostReverseProxy(url)
		handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			r.Header.Add("Refunc-Worker-ID", wid)
			proxy.ServeHTTP(w, r)
		})
		server.Handler = handler
		if err := server.Serve(l); err != nil {
			klog.Errorf("(loader) proxy runtime server %s err %v", wid, err)
		}
	}(listener)

	for i := 0; i < 200; i++ {
		res, err := http.Get("http://" + listener.Addr().String() + "/2018-06-01/ping")
		if err != nil {
			<-time.After(5 * time.Millisecond)
			continue
		}

		defer res.Body.Close()
		body, err := ioutil.ReadAll(res.Body)
		if err != nil || string(body) != "pong" {
			return "", errors.New("loader: failed to reqeust api")
		}
		break
	}

	return listener.Addr().String(), nil
}
