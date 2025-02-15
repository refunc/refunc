/*
Copyright 2025 The refunc Authors

TODO: choose a opensource licence.
*/

// Code generated by client-gen. DO NOT EDIT.

package v1beta3

import (
	"net/http"

	v1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/generated/clientset/versioned/scheme"
	rest "k8s.io/client-go/rest"
)

type RefuncV1beta3Interface interface {
	RESTClient() rest.Interface
	FuncdevesGetter
	FuncinstsGetter
	TriggersGetter
	XenvsGetter
}

// RefuncV1beta3Client is used to interact with features provided by the refunc.refunc.io group.
type RefuncV1beta3Client struct {
	restClient rest.Interface
}

func (c *RefuncV1beta3Client) Funcdeves(namespace string) FuncdefInterface {
	return newFuncdeves(c, namespace)
}

func (c *RefuncV1beta3Client) Funcinsts(namespace string) FuncinstInterface {
	return newFuncinsts(c, namespace)
}

func (c *RefuncV1beta3Client) Triggers(namespace string) TriggerInterface {
	return newTriggers(c, namespace)
}

func (c *RefuncV1beta3Client) Xenvs(namespace string) XenvInterface {
	return newXenvs(c, namespace)
}

// NewForConfig creates a new RefuncV1beta3Client for the given config.
// NewForConfig is equivalent to NewForConfigAndClient(c, httpClient),
// where httpClient was generated with rest.HTTPClientFor(c).
func NewForConfig(c *rest.Config) (*RefuncV1beta3Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	httpClient, err := rest.HTTPClientFor(&config)
	if err != nil {
		return nil, err
	}
	return NewForConfigAndClient(&config, httpClient)
}

// NewForConfigAndClient creates a new RefuncV1beta3Client for the given config and http client.
// Note the http client provided takes precedence over the configured transport values.
func NewForConfigAndClient(c *rest.Config, h *http.Client) (*RefuncV1beta3Client, error) {
	config := *c
	if err := setConfigDefaults(&config); err != nil {
		return nil, err
	}
	client, err := rest.RESTClientForConfigAndClient(&config, h)
	if err != nil {
		return nil, err
	}
	return &RefuncV1beta3Client{client}, nil
}

// NewForConfigOrDie creates a new RefuncV1beta3Client for the given config and
// panics if there is an error in the config.
func NewForConfigOrDie(c *rest.Config) *RefuncV1beta3Client {
	client, err := NewForConfig(c)
	if err != nil {
		panic(err)
	}
	return client
}

// New creates a new RefuncV1beta3Client for the given RESTClient.
func New(c rest.Interface) *RefuncV1beta3Client {
	return &RefuncV1beta3Client{c}
}

func setConfigDefaults(config *rest.Config) error {
	gv := v1beta3.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/apis"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	if config.UserAgent == "" {
		config.UserAgent = rest.DefaultKubernetesUserAgent()
	}

	return nil
}

// RESTClient returns a RESTClient that is used to communicate
// with API server by this client implementation.
func (c *RefuncV1beta3Client) RESTClient() rest.Interface {
	if c == nil {
		return nil
	}
	return c.restClient
}
