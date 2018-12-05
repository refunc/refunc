package operators

import (
	"sync"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/client"
)

// Interface provides functions to manage funcinsts
type Interface interface {
	TriggerForEndpoint(endpoint string) (*rfv1beta3.Trigger, error)
	ResolveFuncdef(trigger *rfv1beta3.Trigger) (*rfv1beta3.Funcdef, error)
	GetFuncInstance(trigger *rfv1beta3.Trigger) (*rfv1beta3.Funcinst, error)
	GetNamespace() string
	Tap(key string)
}

// TaskResolverFactory creates a task resolver
type TaskResolverFactory func() (client.TaskResolver, error)

// LiveTaskStore stores live tasks
type LiveTaskStore interface {
	GetOrCreateTask(id string, factory TaskResolverFactory) (tr client.TaskResolver, created bool, err error)
	Get(id string) (tr client.TaskResolver, has bool, err error)
	Delete(id string)
	Range(func(id string, tr client.TaskResolver) bool)
}

// NewLiveTaskStore returns a new LiveTaskStore
func NewLiveTaskStore() LiveTaskStore {
	return new(liveTaskStore)
}

type liveTaskStore struct {
	tasks sync.Map
}

func (lts *liveTaskStore) GetOrCreateTask(id string, factory TaskResolverFactory) (tr client.TaskResolver, created bool, err error) {
	val, loaded := lts.tasks.LoadOrStore(id, newLazyTaskGetter(factory))
	tr, err = val.(TaskResolverFactory)()
	created = !loaded
	return
}

func (lts *liveTaskStore) Get(id string) (tr client.TaskResolver, has bool, err error) {
	if val, ok := lts.tasks.Load(id); ok {
		tr, err = val.(TaskResolverFactory)()
		has = true
	}
	return
}

func (lts *liveTaskStore) Delete(id string) {
	lts.tasks.Delete(id)
}

func (lts *liveTaskStore) Range(fn func(id string, tr client.TaskResolver) bool) {
	lts.tasks.Range(func(key interface{}, value interface{}) bool {
		if tr, err := value.(TaskResolverFactory)(); err == nil {
			return fn(key.(string), tr)
		}
		return true
	})
}

func newLazyTaskGetter(fn TaskResolverFactory) TaskResolverFactory {
	// local states, make it can lazily initialize
	var (
		once sync.Once
		t    client.TaskResolver
		e    error
	)

	return func() (client.TaskResolver, error) {
		once.Do(func() {
			t, e = fn()
		})
		return t, e
	}
}
