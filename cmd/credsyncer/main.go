package main

import (
	"context"
	"encoding/json"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"time"

	"k8s.io/klog"

	"github.com/garyburd/redigo/redis"
	"github.com/refunc/refunc/pkg/credsyncer"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/utils/cmdutil"
	"github.com/refunc/refunc/pkg/utils/cmdutil/flagtools"
	"github.com/refunc/refunc/pkg/utils/cmdutil/sharedcfg"
	"github.com/spf13/pflag"
)

var config struct {
	Namespace  string
	RemainData bool
}

func init() {
	pflag.StringVarP(&config.Namespace, "namespace", "n", "", "The scope of namepsace to manipulate")
	pflag.BoolVar(&config.RemainData, "remain-data", false, "Do not flush DB before sync")
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())
	rand.Seed(time.Now().UTC().UnixNano())

	flagtools.InitFlags()

	klog.CopyStandardLogTo("INFO")
	defer klog.Flush()

	namespace := os.Getenv("REFUNC_NAMESPACE")
	if len(namespace) == 0 {
		klog.Fatalf("must set env (REFUNC_NAMESPACE)")
	}

	prefix := env.GlobalBucket
	if prefix == "" {
		klog.Fatalf("REFUNC_SCOPE_PREFIX in env must not be empty")
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		klog.Fatalln("REDIS_URL cannot be empty")
	}
	c, err := redis.DialURL(redisURL)
	if err != nil {
		klog.Fatalf("failed to connect to redis")
	}
	defer c.Close()

	if !config.RemainData {
		c.Do("FLUSHDB")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sc := sharedcfg.New(ctx, config.Namespace)
	sc.AddController(func(cfg sharedcfg.Configs) sharedcfg.Runner {
		redstore := newRedisStore(c)
		syncer, err := credsyncer.NewCredSyncer(
			namespace,
			prefix,
			redstore,
			cfg.RefuncInformers(),
			cfg.KubeInformers(),
		)
		if err != nil {
			klog.Fatalf("Failed to start credentials syncer, %v", err)
		}
		return syncer
	})

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		sc.Run(ctx.Done())
	}()

	klog.Infof(`Received signal "%v", exiting...`, <-cmdutil.GetSysSig())

	cancel()
	wg.Wait()
}

type redisStore struct {
	c       redis.Conn
	syncDoC chan func()
}

func newRedisStore(c redis.Conn) *redisStore {
	return &redisStore{
		c:       c,
		syncDoC: make(chan func()),
	}
}

func (r *redisStore) Run(stopC <-chan struct{}) {
	defer close(r.syncDoC)
	for {
		select {
		case <-stopC:
			return
		case fn := <-r.syncDoC:
			fn()
		}
	}
}

func (r *redisStore) AddCreds(creds *credsyncer.FlatCreds) error {
	bts, err := json.Marshal(creds)
	if err != nil {
		return err
	}
	// TODO: set deadline
	_, err = r.redisDo("SET", creds.AccessKey, bts)
	return err
}

func (r *redisStore) DeleteCreds(accessKey string) error {
	_, err := r.redisDo("DEL", accessKey)
	return err
}

func (r *redisStore) redisDo(commandName string, args ...interface{}) (reply interface{}, err error) {
	r.syncDoC <- func() {
		reply, err = r.c.Do(commandName, args...)
	}
	return
}
