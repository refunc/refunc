package httptrigger

import (
	"errors"

	"github.com/allegro/bigcache"
)

type httpCache interface {
	Get(key string) ([]byte, error)
	Set(key string, entry []byte) error
	Reset() error
	Len() int
}

var (
	_ httpCache = (*bigcache.BigCache)(nil)
	_ httpCache = (*disabledCache)(nil)
)

type disabledCache struct{}

var errDisabledCache = errors.New("router: cache disabled")

func (disabledCache) Get(key string) ([]byte, error) {
	return nil, errDisabledCache
}

func (disabledCache) Set(key string, entry []byte) error {
	return nil
}

func (disabledCache) Reset() error {
	return nil
}

func (disabledCache) Len() int {
	return 0
}
