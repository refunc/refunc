package crontrigger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"k8s.io/klog"

	minio "github.com/minio/minio-go"
	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/client"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/messages"
	"github.com/refunc/refunc/pkg/utils"
	"github.com/robfig/cron"
)

type cronHandler struct {
	trKey string
	ns    string
	name  string

	next time.Time

	operator *Operator
}

var tzdata sync.Map

func (t *cronHandler) Next() (next time.Time, err error) {
	trigger, err := t.operator.TriggerLister.Triggers(t.ns).Get(t.name)
	if err != nil {
		return
	}

	tz, ok := tzdata.Load(trigger.Spec.Cron.Location)
	if !ok {
		tz, err = time.LoadLocation(trigger.Spec.Cron.Location)
		if err != nil {
			return
		}
		tzdata.Store(trigger.Spec.Cron.Location, tz)
	}
	now := time.Now().In(tz.(*time.Location))

	if !t.next.IsZero() && t.next.After(now) {
		// trigger's cron is not changed, and trigger is not executed related to `now`, so return last evaluated time.
		// this ensure cron expression with @every x, @hourly works well
		return t.next, nil
	}
	// trigger is fired, or this is the first time to schedule
	sched, err := cron.ParseStandard(trigger.Spec.Cron.Cron)
	if err != nil {
		return
	}
	t.next = sched.Next(now)
	return t.next, nil
}

const (
	s3Prefix      = "_system/triggers/crontrigger"
	logsChunkSize = 4<<(10*2) + 512<<10 // 4.5 MB
)

func (t *cronHandler) Run(tm time.Time) {
	trigger, err := t.operator.TriggerLister.Triggers(t.ns).Get(t.name)
	if err != nil {
		klog.Errorf("(h) %s failed to get trigger, %v", t.trKey, err)
		return
	}

	fndef, err := t.operator.ResolveFuncdef(trigger)
	if err != nil {
		klog.Errorf("(h) %s failed to get fundef, %v", t.trKey, err)
		return
	}
	ts := tm.Truncate(time.Second).Format(time.RFC3339)
	taskr, created, err := t.ensureTask(fndef, trigger, ts)
	if err != nil {
		klog.Errorf("(h) failed to start task %s, %v", t.trKey, err)
		return
	}
	if !created {
		klog.Warningf("(h) %s is already started", taskr.ID())
		return
	}

	// start and polling
	go func() {
		klog.Warningf("(h) %s started", taskr.ID())
		defer func() {
			re := recover()
			t.operator.liveTasks.Delete(taskr.ID())
			if re != nil {
				utils.LogTraceback(re, 5, klog.V(1))
				err = fmt.Errorf("h: %v", re)
			}
		}()

		logsteam := taskr.LogObserver()
		logNameSuffix := 0
		logsName := func() string {
			suffix := ""
			if logNameSuffix > 0 {
				suffix = fmt.Sprintf(".%d", logNameSuffix)
			}
			logNameSuffix++
			return env.KeyWithinScope(filepath.Join(
				s3Prefix,
				t.ns,
				t.name,
				"logs",
				fmt.Sprintf("%s%s.log", ts, suffix),
			))
		}
		writeLogs := func(lines []byte) {
			if len(lines) == 0 {
				return
			}
			key := logsName()
			_, err := env.GlobalMinioClient().PutObject(
				env.GlobalBucket, key,
				bytes.NewReader(lines), int64(len(lines)),
				minio.PutObjectOptions{ContentType: "text/plain; charset=UTF-8"},
			)
			if err != nil {
				klog.Errorf("(h) %s failed to write logs %q, %v", taskr.ID(), key, err)
			}
		}

		var lines []byte
		defer func() { writeLogs(lines) }()
		for {
			select {
			case <-logsteam.Changes():
				for logsteam.HasNext() {
					lines = append(lines, []byte(logsteam.Next().(string))...)
					lines = append(lines, messages.TokenCRLF...)
				}
				if len(lines) > logsChunkSize {
					writeLogs(lines)
					lines = lines[0:0]
				}

			case <-taskr.Done():
				bts, err := taskr.Result()
				if err != nil {
					klog.Errorf("(h) %s failed, %v", taskr.ID(), err)
					bts = messages.GetErrActionBytes(err)
				}
				// write result
				key := env.KeyWithinScope(filepath.Join(
					s3Prefix,
					t.ns,
					t.name,
					"results",
					fmt.Sprintf("%s.json", ts),
				))
				_, err = env.GlobalMinioClient().PutObject(
					env.GlobalBucket, key,
					bytes.NewReader(bts), int64(len(bts)),
					minio.PutObjectOptions{ContentType: "text/plain; charset=UTF-8"},
				)
				if err != nil {
					klog.Errorf("(h) %s failed to write results %q, %v", taskr.ID(), key, err)
				}
				return
			}
		}
	}()
}

const codeLaunchBias = 10 * time.Second

// ensureTask gets or creates a client.TaskResolver
func (t *cronHandler) ensureTask(fndef *rfv1beta3.Funcdef, trigger *rfv1beta3.Trigger, ts string) (client.TaskResolver, bool, error) {
	id := t.trKey + "@" + ts
	return t.operator.liveTasks.GetOrCreateTask(id, func() (_ client.TaskResolver, err error) {
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

		var args map[string]interface{}
		if trigger.Spec.Cron.Args != nil {
			// render request data
			err = json.Unmarshal(trigger.Spec.Cron.Args, &args)
			if err != nil {
				return
			}
		}
		if len(args) == 0 {
			args = make(map[string]interface{})
		}
		args["$time"] = ts
		args["$triggerName"] = trigger.Name

		endpoint := trigger.Namespace + "/" + trigger.Spec.FuncName
		request := &messages.InvokeRequest{
			Args:      messages.MustFromObject(args),
			RequestID: id,
		}
		var timeout = messages.DefaultJobTimeout
		if fndef.Spec.Runtime != nil && fndef.Spec.Runtime.Timeout > 0 {
			timeout = time.Second*time.Duration(fndef.Spec.Runtime.Timeout) + codeLaunchBias
		}

		ctx := t.operator.ctx
		ctx = client.WithLogger(ctx, klog.V(1))
		ctx = client.WithTimeoutHint(ctx, timeout)
		ctx = client.WithLoggingHint(ctx, true)
		return client.NewTaskResolver(ctx, endpoint, request)
	})
}
