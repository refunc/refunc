package credsyncer

import (
	"encoding/json"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	rfv1beta3 "github.com/refunc/refunc/pkg/apis/refunc/v1beta3"
	rfinformers "github.com/refunc/refunc/pkg/generated/informers/externalversions"
)

const (
	// CredentialConfigAnnotation is annotation key for credential configs
	CredentialConfigAnnotation = "refunc.io/is-credential-config"
)

// Syncer sync credentials from refunc and provide for storage layer
type Syncer interface {
	Run(stopC <-chan struct{})
}

// Store is storage interface to manage creds
type Store interface {
	AddCreds(creds *FlatCreds) error
	DeleteCreds(accessKey string) error
}

// FlatCreds is flat verison of creds and permission
type FlatCreds struct {
	// ns/name if it comes from a funcinst
	FuncinstID string `json:"funcinst,omitempty"`

	// meta
	ID        string `json:"id,omitempty"`
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`

	// storage
	Scope string `json:"scope,omitempty"`

	// network
	Permissions struct {
		Publish   []string `json:"publish,omitempty"`
		Subscribe []string `json:"subscribe,omitempty"`
	} `json:"permissions"`
}

// NewCreds creates new flat creds from a valid funcinst
func NewCreds(fni *rfv1beta3.Funcinst, prefix string) *FlatCreds {
	creds := &FlatCreds{
		ID:        fni.Namespace + "/" + fni.Name,
		AccessKey: fni.Spec.Runtime.Credentials.AccessKey,
		SecretKey: fni.Spec.Runtime.Credentials.SecretKey,
	}
	scope := fni.Spec.Runtime.Permissions.Scope
	// ensure scope always within in a folder
	creds.Scope = strings.TrimRight(filepath.Join("/", prefix, scope), "/") + "/"
	creds.Permissions.Publish = fni.Spec.Runtime.Permissions.Publish
	creds.Permissions.Subscribe = fni.Spec.Runtime.Permissions.Subscribe
	return creds
}

// credSyncer syncs and manages funcinsts.
type credSyncer struct {
	ns string

	prefix string

	store Store

	kubeInformers   k8sinformers.SharedInformerFactory
	refuncInformers rfinformers.SharedInformerFactory

	wantedInformers []cache.InformerSynced
}

// NewCredSyncer creates a credential provider
func NewCredSyncer(
	namespace,
	prefix string,
	store Store,
	refuncInformers rfinformers.SharedInformerFactory,
	kubeInformers k8sinformers.SharedInformerFactory,
) (Syncer, error) {
	r := &credSyncer{
		ns:              namespace,
		prefix:          prefix,
		store:           store,
		refuncInformers: refuncInformers,
		kubeInformers:   kubeInformers,
	}

	r.wantedInformers = []cache.InformerSynced{
		r.refuncInformers.Refunc().V1beta3().Funcinsts().Informer().HasSynced,
		r.kubeInformers.Core().V1().ConfigMaps().Informer().HasSynced,
	}

	return r, nil
}

// Run will not return until stopC is closed.
func (r *credSyncer) Run(stopC <-chan struct{}) {
	klog.Info("(credsyncer) starting")
	// add events emitter
	r.refuncInformers.Refunc().V1beta3().Funcinsts().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleFuncinstAdd,
		UpdateFunc: r.handleFuncinstUpdate,
		DeleteFunc: r.handleFuncinstDelete,
	})
	r.kubeInformers.Core().V1().ConfigMaps().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    r.handleConfigAdd,
		UpdateFunc: r.handleConfigUpdate,
		DeleteFunc: r.handleConfigDelete,
	})

	klog.Info("(credsyncer) waiting for listers to be fully synced")
	if !cache.WaitForCacheSync(stopC, r.wantedInformers...) {
		klog.Error("(credsyncer) failed to start")
		return
	}
	klog.Info("(credsyncer) listers has fully synced")
}

func (r *credSyncer) handleFuncinstAdd(o interface{}) {
	fni := o.(*rfv1beta3.Funcinst)
	if !fni.Status.IsActiveCondition() {
		return
	}
	if key := fni.Spec.Runtime.Credentials.AccessKey; key != "" {
		ns, name := getNsName(fni)
		klog.V(3).Infof("(credsyncer) synced credential for %s/%s", ns, name)

		if err := r.addCred(NewCreds(fni, r.prefix)); err != nil {
			klog.Errorf("(credsyncer) failed to set cred in redis %s/%s, %v", ns, name, err)
		} else {
			klog.Infof("(credsyncer) set cred %s/%s for funcinst", ns, name)
		}
	}
}

func (r *credSyncer) handleFuncinstDelete(o interface{}) {
	fni, ok := o.(*rfv1beta3.Funcinst)
	if !ok {
		// it's cache.DeletedFinalStateUnknown
		return
	}
	if key := fni.Spec.Runtime.Credentials.AccessKey; key != "" {
		ns, name := getNsName(fni)
		klog.V(3).Infof("(credsyncer) removing credential for funcinst %s/%s", ns, name)
		if err := r.deleteCred(key); err != nil {
			klog.Errorf("(credsyncer) failed to del cred in redis %s/%s, %v", ns, name, err)
		}
	}
}

func (r *credSyncer) handleFuncinstUpdate(oldObj, curObj interface{}) {
	old := oldObj.(*rfv1beta3.Funcinst)
	cur := curObj.(*rfv1beta3.Funcinst)

	// Periodic resync may resend the deployment without changes in-between.
	// Also breaks loops created by updating the resource ourselves.
	if old.ResourceVersion == cur.ResourceVersion {
		return
	}

	if rfv1beta3.OnlyLastActivityChanged(old, cur) {
		return
	}

	accessKey := old.Spec.Runtime.Credentials.AccessKey
	newAccessKey, newSecretKey := cur.Spec.Runtime.Credentials.AccessKey, cur.Spec.Runtime.Credentials.SecretKey

	if !cur.Status.IsActiveCondition() {
		r.handleFuncinstDelete(old)
		return
	}

	if accessKey != newAccessKey {
		// drop old
		r.handleFuncinstDelete(old)
	}
	if newAccessKey != "" && newSecretKey != "" {
		// override
		r.handleFuncinstAdd(cur)
	}
}

func (r *credSyncer) handleConfigAdd(o interface{}) {
	if !r.checkAnno(o) {
		return
	}
	cfg := o.(*corev1.ConfigMap)
	ns, name := getNsName(cfg)
	accessKey, ok := cfg.Data["accessKey"]
	if !ok {
		return
	}
	secretKey, ok := cfg.Data["secretKey"]
	if !ok {
		return
	}
	scope := cfg.Data["scope"]
	id := cfg.Data["id"]
	if id == "" {
		// try username
		id = cfg.Data["user"]
	}
	if id == "" {
		id = ns + "/" + name
	}
	// check permissions
	var perms rfv1beta3.Permissions
	if permsStr, ok := cfg.Data["permissions"]; ok {
		if err := json.Unmarshal([]byte(permsStr), &perms); err != nil {
			klog.Errorf("(credsyncer) failed to parse permissions for %q", id)
		}
	}
	if perms.Scope != "" {
		// override scope
		scope = perms.Scope
	}
	creds := &FlatCreds{
		ID:        id,
		AccessKey: accessKey,
		SecretKey: secretKey,
	}
	// ensure scope always within in a folder
	creds.Scope = strings.TrimRight(filepath.Join("/", r.prefix, scope), "/") + "/"
	creds.Permissions.Publish = perms.Publish
	creds.Permissions.Subscribe = perms.Subscribe
	if err := r.addCred(creds); err != nil {
		klog.Errorf("(credsyncer) failed to set cred in redis %q, %v", id, err)
	} else {
		klog.Infof("(credsyncer) set cred %q for configmap", id)
	}
}

func (r *credSyncer) handleConfigDelete(o interface{}) {
	if !r.checkAnno(o) {
		return
	}
	cfg := o.(*corev1.ConfigMap)
	accessKey := cfg.Data["accessKey"]
	if accessKey != "" {
		return
	}
	ns, name := getNsName(cfg)
	klog.V(3).Infof("(credsyncer) removing credential for configmap %s/%s", ns, name)
	if err := r.deleteCred(accessKey); err != nil {
		klog.Errorf("(credsyncer) failed to del cred in redis %s/%s, %v", ns, name, err)
	}
}

func (r *credSyncer) handleConfigUpdate(oldObj, curObj interface{}) {
	if !r.checkAnno(oldObj) {
		return
	}
	old := oldObj.(*corev1.ConfigMap)
	cur := curObj.(*corev1.ConfigMap)

	// Periodic resync may resend the deployment without changes in-between.
	// Also breaks loops created by updating the resource ourselves.
	if old.ResourceVersion == cur.ResourceVersion {
		return
	}

	accessKey := old.Data["accessKey"]
	newAccessKey, newSecretKey := cur.Data["accessKey"], cur.Data["secretKey"]

	if accessKey != newAccessKey {
		// drop old
		r.handleConfigDelete(old)
	}
	if newAccessKey != "" && newSecretKey != "" {
		// override
		r.handleConfigAdd(cur)
	}
}

func (r *credSyncer) addCred(creds *FlatCreds) error {
	return r.store.AddCreds(creds)
}

func (r *credSyncer) deleteCred(accessKey string) error {
	return r.store.DeleteCreds(accessKey)
}

func (r *credSyncer) checkAnno(o interface{}) bool {
	obj, ok := o.(metav1.Object)
	if !ok {
		return false
	}
	// contraint namespace
	if r.ns != "" && obj.GetNamespace() != r.ns {
		return false
	}
	anno := obj.GetAnnotations()
	if val, has := anno[CredentialConfigAnnotation]; has && strings.ToLower(val) == "true" {
		return true
	}
	return false
}

func getNsName(obj metav1.Object) (ns, name string) {
	ns, name = obj.GetNamespace(), obj.GetName()
	return
}
