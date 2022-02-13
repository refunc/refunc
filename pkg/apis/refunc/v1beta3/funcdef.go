package v1beta3

import (
	"encoding/json"
	"errors"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// CRD names for Funcdef
const (
	FuncdefKind       = "Funcdef"
	FuncdefPluralName = "funcdeves"
)

// static asserts
var (
	_ runtime.Object            = (*Funcdef)(nil)
	_ metav1.ObjectMetaAccessor = (*Funcdef)(nil)

	_ runtime.Object          = (*FuncdefList)(nil)
	_ metav1.ListMetaAccessor = (*FuncdefList)(nil)
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Funcdef is a API object to represent a FUNCtion DEFinition
type Funcdef struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec FuncdefSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FuncdefList is a API object to represent a list of Refuncs
type FuncdefList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Funcdef `json:"items"`
}

// FuncdefSpec is the specification to describe a Funcdef
type FuncdefSpec struct {
	// storage path for function
	Body string `json:"body,omitempty"`
	// unique hash that can identify current function
	Hash string `json:"hash"`
	// The entry name to execute when a function is activated
	Entry string `json:"entry,omitempty"`
	// the min number of provisioned executors
	// optional, 0 means do not provisioned
	MinReplicas int32 `json:"minReplicas,omitempty"`
	// the maximum number of parallel executors
	// optional, 0 means do not scale
	MaxReplicas int32 `json:"maxReplicas,omitempty"`
	// Runtime options for agent and runtime builder
	Runtime *Runtime `json:"runtime"`
	// Meta backport for legacy
	Meta json.RawMessage `json:"meta,omitempty"`
	// Custom any extra or experiments put in here
	Custom json.RawMessage `json:"custom,omitempty"`
}

// Runtime runtime to operate this template
type Runtime struct {
	// name of xenv
	Name string `json:"name,omitempty"`

	Envs    map[string]string `json:"envs,omitempty"`
	Timeout int               `json:"timeout,omitempty"`
}

// ErrUnknownTriggerType indicates that we cannot processed the given triger sepc
var ErrUnknownTriggerType = errors.New("refunc: got unknown funcinst type")

// AsOwner returns *metav1.OwnerReference
func (fn *Funcdef) AsOwner() *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: APIVersion,
		Kind:       FuncdefKind,
		Name:       fn.Name,
		UID:        fn.UID,
		Controller: &trueVar,
	}
}

// Ref returns *corev1.ObjectReference
func (fn *Funcdef) Ref() *corev1.ObjectReference {
	if fn == nil {
		return nil
	}
	return &corev1.ObjectReference{
		APIVersion: APIVersion,
		Kind:       FuncdefKind,
		Namespace:  fn.Namespace,
		Name:       fn.Name,
		UID:        fn.UID,
	}
}
