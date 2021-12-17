package funcinst

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	"github.com/refunc/refunc/pkg/runtime"
	"github.com/refunc/refunc/pkg/utils/k8sutil"
	"github.com/refunc/refunc/pkg/utils/rfutil"
)

const updateLabelRetries = 1

func (rc *Controller) handlePodChange(o interface{}) {
	if obj, ok := rfutil.IsExecutorRes(o); ok {
		pod := obj.(*corev1.Pod)
		fniName, ok := pod.GetLabels()[rfv1beta3.LabelName]
		if !ok {
			klog.Warningf("(tc) got executor pod %s/%s without funcinst's name", pod.Namespace, pod.Name)
			return
		}
		fni, err := rc.funcinstLister.Funcinsts(pod.Namespace).Get(fniName)
		if err != nil {
			if pod.DeletionTimestamp == nil {
				klog.Errorf("(tc) cannot resolve funcinst %s/%s, %v", pod.Namespace, fniName, err)
			}
			return
		}
		if fni.Status.IsInactiveCondition() {
			// do not weakup when funcinst is inactive
			return
		}
		// if current funcinst is not acitve, we should fillter out pods in bad condition.
		// once a funcinst is active, any changes upon related pod should wakeup controller.
		if !fni.Status.IsActiveCondition() {
			running, _ := k8sutil.PodRunningAndReady(*pod)
			if !running || rfutil.IsRuntimePodReady(pod) {
				// skip pod which is not ready or has been initialized already
				return
			}
		}

		rc.enqueue(fni, "Pod Change")
	}
}

func (rc *Controller) relabelPodFromDeployment(fni *rfv1beta3.Funcinst, dep *appsv1.Deployment) (*corev1.Pod, error) {
	var pod *corev1.Pod

	// select a running pod
	pods, err := rc.podLister.Pods(dep.Namespace).List(labels.Set(dep.Spec.Selector.MatchLabels).AsSelectorPreValidated())
	if err != nil {
		return nil, err
	}
	for i := range pods {
		running, _ := k8sutil.PodRunningAndReady(*pods[i])
		if running {
			pod = pods[i]
			break
		}
	}
	if pod == nil {
		return nil, nil
	}

	// update labels
	pod = pod.DeepCopy()
	for i := 0; ; i++ {
		if len(pod.Labels) == 0 {
			pod.Labels = make(map[string]string)
		}
		for k, v := range rfutil.ExecutorLabels(fni) {
			pod.Labels[k] = v
		}
		// set pod to uninitialized explicitly
		pod.Labels[rfv1beta3.LabelExecutorIsReady] = "false"
		// set owner referneces to nil in order to adpot by executor rs
		pod.OwnerReferences = nil
		pod, err = rc.kclient.CoreV1().Pods(fni.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
		if i < updateLabelRetries && err != nil {
			// get pod from upstream, and try again
			pod, err = rc.kclient.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
			if err != nil {
				// really bad happened
				return nil, err
			}
			continue
		}
		return pod, err
	}
}

func (rc *Controller) initRuntimePod(fni *rfv1beta3.Funcinst, fndef *rfv1beta3.Funcdef, xenv *rfv1beta3.Xenv, rt runtime.Interface, pod *corev1.Pod) error {
	t0 := time.Now()
	if !rt.IsPodReady(pod) {
		return fmt.Errorf("rc: pod %q is not ready", rfutil.ExecutorPodName(pod))
	}
	if latest, e := rc.kclient.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{}); e == nil {
		// retrive the latest object, to avoid re-init
		pod = latest
	}
	// maybe cache is not synced, the pod has been inited
	if rfutil.IsRuntimePodReady(pod) {
		return nil
	}

	// prepare a temp working dir
	dir, err := ioutil.TempDir("", rt.Name())
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	klog.V(4).Infof("(tc) create tmp working dir %q for runner", dir)
	if err := rt.InitPod(pod, fni, fndef, xenv, dir); err != nil {
		return err
	}

	// update labels
	pod = pod.DeepCopy()
	for i := 0; ; i++ {
		pod.Labels[rfv1beta3.LabelExecutorIsReady] = "true"
		_, err := rc.kclient.CoreV1().Pods(pod.Namespace).Update(context.TODO(), pod, metav1.UpdateOptions{})
		if i < updateLabelRetries && err != nil {
			// get pod from upstream, and try again
			pod, err = rc.kclient.CoreV1().Pods(pod.Namespace).Get(context.TODO(), pod.Name, metav1.GetOptions{})
			if err != nil {
				// really bad happened
				return err
			}
			continue
		}
		if err == nil {
			klog.V(2).Infof("(tc) pod %q for %q inited using %v", pod.Name, fni.Namespace+"/"+fni.Name, time.Since(t0))
		}
		return err
	}
}

func (rc *Controller) listPodsForFuncinst(fni *rfv1beta3.Funcinst, fromApiserver bool) ([]*corev1.Pod, error) {
	var lister func(labels.Set) ([]*corev1.Pod, error)
	if fromApiserver {
		lister = func(s labels.Set) ([]*corev1.Pod, error) {
			pl, err := rc.kclient.CoreV1().Pods(fni.Namespace).List(context.TODO(), metav1.ListOptions{
				LabelSelector: s.String(),
			})
			if err != nil {
				return nil, err
			}
			pods := make([]*corev1.Pod, len(pl.Items))
			for i := range pl.Items {
				pods[i] = &pl.Items[i]
			}
			return pods, nil
		}
	} else {
		lister = func(s labels.Set) ([]*corev1.Pod, error) {
			return rc.podLister.Pods(fni.Namespace).List(s.AsSelectorPreValidated())
		}
	}

	pods, err := lister(labels.Set(rfutil.ExecutorLabels(fni)))
	if err != nil {
		return nil, err
	}

	// skip pod in deletion
	var filteredPods []*corev1.Pod
	for _, pod := range pods {
		if pod.DeletionTimestamp == nil {
			filteredPods = append(filteredPods, pod)
		}
	}
	return filteredPods, nil
}
