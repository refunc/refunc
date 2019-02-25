package httptrigger

import (
	"fmt"
	"time"

	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/client"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
)

const codeLaunchBias = 10 * time.Second

// ensureTask gets or creates a client.TaskResolver
func (t *httpHandler) ensureTask(fndef *rfv1beta3.Funcdef, trigger *rfv1beta3.Trigger, request *messages.InvokeRequest) (client.TaskResolver, error) {
	id := request.RequestID
	tr, _, retErr := t.operator.liveTasks.GetOrCreateTask(id, func() (_ client.TaskResolver, err error) {
		defer func() {
			re := recover()
			if err != nil || re != nil {
				t.operator.liveTasks.Delete(id)
			}
			if re != nil {
				utils.LogTraceback(re, 5, klog.V(1))
				err = fmt.Errorf("h: %v", re)
			}
		}()

		// parse job max timeout for a running job
		var timeout = messages.DefaultJobTimeout
		if fndef.Spec.Runtime != nil && fndef.Spec.Runtime.Timeout > 0 {
			timeout = time.Second*time.Duration(fndef.Spec.Runtime.Timeout) + codeLaunchBias
		}

		// rewrite logging option
		fwdLogs := func() bool {
			if v, ok := request.Options["logging"]; ok {
				delete(request.Options, "logging")
				cached, ok := v.(bool)
				return ok && cached
			}
			return false
		}()

		ctx := t.operator.ctx
		ctx = client.WithLogger(ctx, klog.V(2))
		ctx = client.WithTimeoutHint(ctx, timeout)
		ctx = client.WithLoggingHint(ctx, fwdLogs)
		endpoint := trigger.Namespace + "/" + trigger.Spec.FuncName
		tr, err := client.NewTaskResolver(ctx, endpoint, request)
		if err != nil {
			return
		}

		// find if request is enabeld cache
		cacheEnabled := func() bool {
			if v, ok := request.Options["cachePreferred"]; ok {
				cached, ok := v.(bool)
				return ok && cached
			}
			return false
		}()

		// handle task result
		go func() {
			defer func() {
				t.operator.liveTasks.Delete(id)
				klog.V(3).Infof("(h) %s finished", tr.Name())
			}()
			<-tr.Done()
			bts, err := tr.Result()
			if cacheEnabled && err == nil {
				klog.V(3).Infof("(h) %s set cache", tr.Name())
				t.operator.http.cache.Set(id, bts) //nolint:errcheck
			}
		}()

		return tr, nil
	})

	return tr, retErr
}
