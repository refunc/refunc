package v1beta3

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CRD names for Runner templates
const (
	XenvKind       = "Xenv"
	XenvPluralName = "xenvs"
)

var (
	_ runtime.Object            = (*Xenv)(nil)
	_ metav1.ObjectMetaAccessor = (*Xenv)(nil)

	_ runtime.Object          = (*XenvList)(nil)
	_ metav1.ListMetaAccessor = (*XenvList)(nil)
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=xenvs,singular=xenv,shortName=xe
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Xenv is a API object to represent a contaniner based eXecution ENVironment for a function
type Xenv struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec XenvSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// XenvList is a API object to represent a list of Xenv
type XenvList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Xenv `json:"items"`
}

// XenvSpec is the specification to describe a runner
type XenvSpec struct {
	// Name of runtime, default is agent mode
	Type string `json:"type,omitempty"`
	// Name of transport, default is agent mode
	Transport string `json:"transport,omitempty"`

	// Container spec https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.10/#container-v1-core
	Container XenvContainer `json:"container"`

	// SideContainers
	SideContainers []corev1.Container `json:"sideContainers,omitempty"`

	// InitContainers
	InitContainers []corev1.Container `json:"initContainers,omitempty"`

	// Secrets to pull image
	ImagePullSecrets []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`

	// Volume sepc // https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.10/#volume-v1-core
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// Number of pods pre-allocated for(maybe) boosting the speed of a cold start
	PoolSize int `json:"poolSize,omitempty"`

	// ServiceAccount attach to xevn dep
	ServiceAccount string `json:"serviceAccount,omitempty"`

	// A key used for runtime builder to access the shell
	SetupKey string `json:"key,omitempty"`

	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Extra json.RawMessage `json:"extra,omitempty"`
}

type XenvContainer struct {
	Image           string                      `json:"image" protobuf:"bytes,2,opt,name=image"`
	Command         []string                    `json:"command,omitempty" protobuf:"bytes,3,rep,name=command"`
	Env             []corev1.EnvVar             `json:"env,omitempty" patchStrategy:"merge" patchMergeKey:"name" protobuf:"bytes,7,rep,name=env"`
	Resources       corev1.ResourceRequirements `json:"resources,omitempty" protobuf:"bytes,8,opt,name=resources"`
	VolumeMounts    []corev1.VolumeMount        `json:"volumeMounts,omitempty" patchStrategy:"merge" patchMergeKey:"mountPath" protobuf:"bytes,9,rep,name=volumeMounts"`
	ImagePullPolicy corev1.PullPolicy           `json:"imagePullPolicy,omitempty" protobuf:"bytes,14,opt,name=imagePullPolicy,casttype=PullPolicy"`
	SecurityContext *corev1.SecurityContext     `json:"securityContext,omitempty" protobuf:"bytes,15,opt,name=securityContext"`
}

func (c *XenvContainer) DeepCopyContainer() *corev1.Container {
	in := c.DeepCopy()
	return &corev1.Container{
		Name:            "body",
		Image:           in.Image,
		Command:         in.Command,
		Env:             in.Env,
		Resources:       in.Resources,
		VolumeMounts:    in.VolumeMounts,
		ImagePullPolicy: in.ImagePullPolicy,
		SecurityContext: in.SecurityContext,
	}
}

// AsOwner returns *metav1.OwnerReference
func (env *Xenv) AsOwner() *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: APIVersion,
		Kind:       XenvKind,
		Name:       env.Name,
		UID:        env.UID,
		Controller: &trueVar,
	}
}

// Ref returns *corev1.ObjectReference
func (env *Xenv) Ref() *corev1.ObjectReference {
	if env == nil {
		return nil
	}
	return &corev1.ObjectReference{
		APIVersion: APIVersion,
		Kind:       XenvKind,
		Namespace:  env.Namespace,
		Name:       env.Name,
		UID:        env.UID,
	}
}
