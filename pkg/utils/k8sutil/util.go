// Copyright 2016 The prometheus-operator Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// copied from https://github.com/coreos/prometheus-operator/blob/master/pkg/k8sutil/k8sutil.go

package k8sutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/api/extensions/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/strategicpatch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	clientv1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog"
)

// BuildClusterConfig returns a config from masterUrl or kubeconfigPath,
// kubeconfigPath default is ~/.kube/config
func BuildClusterConfig(masterURL, kubeconfigPath string) (*rest.Config, error) {
	if kubeconfigPath == "" {
		kubeconfigPath = filepath.Join(homedir.HomeDir(), ".kube/config")
		if _, err := os.Stat(kubeconfigPath); err != nil {
			// fallback to guess config using InClusterConfig
			kubeconfigPath = ""
		}
	}

	return clientcmd.BuildConfigFromFlags(masterURL, kubeconfigPath)
}

// PodRunningAndReady returns whether a pod is running and each container has passed it's ready state.
func PodRunningAndReady(pod corev1.Pod) (bool, error) {
	if pod.DeletionTimestamp != nil {
		return false, fmt.Errorf("pod deleted")
	}

	switch pod.Status.Phase {
	case corev1.PodFailed, corev1.PodSucceeded:
		return false, fmt.Errorf("pod completed")
	case corev1.PodRunning:
		for _, cond := range pod.Status.Conditions {
			if cond.Type != corev1.PodReady {
				continue
			}
			return cond.Status == corev1.ConditionTrue, nil
		}
		return false, fmt.Errorf("pod ready condition not found")
	}
	return false, nil
}

// IsResourceNotFoundError checks if it is a NotFoundError
func IsResourceNotFoundError(err error) bool {
	se, ok := err.(*apierrors.StatusError)
	if !ok {
		return false
	}
	if se.Status().Code == http.StatusNotFound && se.Status().Reason == metav1.StatusReasonNotFound {
		return true
	}
	return false
}

func CreatePatch(o, n, datastruct interface{}) ([]byte, error) {
	oldData, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}
	newData, err := json.Marshal(n)
	if err != nil {
		return nil, err
	}
	if bytes.Compare(oldData, newData) == 0 {
		return nil, nil
	}
	return strategicpatch.CreateTwoWayMergePatch(oldData, newData, datastruct)
}

func PatchDeployment(kubecli kubernetes.Interface, namespace, name string, updateFunc func(*v1beta1.Deployment)) error {
	od, err := kubecli.ExtensionsV1beta1().Deployments(namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		return err
	}
	nd := od.DeepCopy()
	updateFunc(nd)
	patchData, err := CreatePatch(od, nd, v1beta1.Deployment{})
	if err != nil {
		return err
	}
	if len(patchData) == 0 {
		return nil
	}
	_, err = kubecli.ExtensionsV1beta1().Deployments(namespace).Patch(name, types.StrategicMergePatchType, patchData)
	return err
}

func CascadeDeleteOptions(gracePeriodSeconds int64) *metav1.DeleteOptions {
	return &metav1.DeleteOptions{
		GracePeriodSeconds: func(t int64) *int64 { return &t }(gracePeriodSeconds),
		PropagationPolicy: func() *metav1.DeletionPropagation {
			foreground := metav1.DeletePropagationForeground
			return &foreground
		}(),
	}
}

// CreateOrUpdateService creates or updates the given service
func CreateOrUpdateService(sclient clientv1.ServiceInterface, svc *corev1.Service) error {
	service, err := sclient.Get(svc.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("k8sutil: retrieving service object failed, %v", err)
	}

	if apierrors.IsNotFound(err) {
		_, err = sclient.Create(svc)
		if err != nil {
			return fmt.Errorf("k8sutil: creating service object failed, %v", err)
		}
	} else {
		svc.ResourceVersion = service.ResourceVersion
		svc.Spec.ClusterIP = service.Spec.ClusterIP // since clusterIP is immutable
		_, err := sclient.Update(svc)
		if err != nil && !apierrors.IsNotFound(err) {
			return fmt.Errorf("k8sutil: updating service object failed, %v", err)
		}
	}

	return nil
}

// CreateOrUpdateEndpoints creates or updates the endpoints
func CreateOrUpdateEndpoints(eclient clientv1.EndpointsInterface, eps *corev1.Endpoints) error {
	endpoints, err := eclient.Get(eps.Name, metav1.GetOptions{})
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("k8sutil: retrieving existing kubelet service object failed, %v", err)
	}

	if apierrors.IsNotFound(err) {
		_, err = eclient.Create(eps)
		if err != nil {
			return fmt.Errorf("k8sutil: creating kubelet enpoints object failed, %v", err)
		}
	} else {
		eps.ResourceVersion = endpoints.ResourceVersion
		_, err = eclient.Update(eps)
		if err != nil {
			return fmt.Errorf("k8sutil: updating kubelet enpoints object failed, %v", err)
		}
	}

	return nil
}

// CreateRecorder returns a new event recorder
func CreateRecorder(kubecli kubernetes.Interface, name, namespace string) record.EventRecorder {
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.Infof)
	eventBroadcaster.StartRecordingToSink(&clientv1.EventSinkImpl{Interface: clientv1.New(kubecli.Core().RESTClient()).Events(namespace)})
	return eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: name})
}
