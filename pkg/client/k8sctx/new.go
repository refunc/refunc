package k8sctx

import (
	"context"
	"fmt"

	nats "github.com/nats-io/nats.go"
	refuncclient "github.com/refunc/refunc/pkg/client"
	"github.com/refunc/refunc/pkg/env"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

func New(parent context.Context, namespace string, client kubernetes.Interface, config *rest.Config) (context.Context, context.CancelFunc, error) {
	podName, err := getNatsPodName(client.CoreV1(), namespace)
	if err != nil {
		return nil, nil, err
	}
	const natsPort = 4222
	t := k8sutil.NewTunnel(client.CoreV1().RESTClient(), config, namespace, podName, natsPort)
	if err := t.ForwardPort(); err != nil {
		return nil, nil, err
	}
	name := refuncclient.Name(parent)
	if name == "" {
		name = "k8s_discovered/" + namespace
	}
	natsConn, err := env.NewNatsConn(nats.Name(name))
	if err != nil {
		t.Close()
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(parent)
	ctx = refuncclient.WithNatsConn(ctx, natsConn)
	cancelFn := func() {
		cancel()
		t.Close()
	}

	return ctx, cancelFn, nil
}

var (
	natsPodLabels = labels.Set{"refunc.io/res": "message", "refunc.io/name": "nats"}
)

func getNatsPodName(client corev1.PodsGetter, namespace string) (string, error) {
	selector := natsPodLabels.AsSelector()
	pod, err := getFirstRunningPod(client, namespace, selector)
	if err != nil {
		return "", err
	}
	return pod.ObjectMeta.GetName(), nil
}

func getFirstRunningPod(client corev1.PodsGetter, namespace string, selector labels.Selector) (*v1.Pod, error) {
	options := metav1.ListOptions{LabelSelector: selector.String()}
	pods, err := client.Pods(namespace).List(context.TODO(), options)
	if err != nil {
		return nil, err
	}
	if len(pods.Items) < 1 {
		return nil, fmt.Errorf("could not find nats")
	}
	for _, p := range pods.Items {
		if ready, _ := k8sutil.PodRunningAndReady(p); ready {
			return &p, nil
		}
	}
	return nil, fmt.Errorf("could not find a ready tiller pod")
}
