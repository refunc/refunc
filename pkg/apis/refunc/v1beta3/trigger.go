package v1beta3

import (
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// CRD names for Funcdef
const (
	TriggerKind       = "Trigger"
	TriggerPluralName = "triggers"
)

// static asserts
var (
	_ runtime.Object            = (*Trigger)(nil)
	_ metav1.ObjectMetaAccessor = (*Trigger)(nil)

	_ runtime.Object          = (*TriggerList)(nil)
	_ metav1.ListMetaAccessor = (*TriggerList)(nil)
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Trigger is a API object to represent a FUNCtion DEClaration
type Trigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec TriggerSpec `json:"spec"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// TriggerList is a API object to represent a list of Refuncs
type TriggerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Trigger `json:"items"`
}

// TriggerSpec is the specification that describes a funcinst for refunc
type TriggerSpec struct {
	FuncName string `json:"funcName"`
	Type     string `json:"type"`

	TriggerConfig `json:",inline"`
}

// TriggerConfig is configuraion for a specific trigger
type TriggerConfig struct {
	Event       *EventTrigger `json:"event,omitempty"`
	Cron        *CronTrigger  `json:"cron,omitempty"`
	HTTPTrigger *HTTPTrigger  `json:"http,omitempty"`
}

// EventTrigger is a basic trigger for a funcdef
type EventTrigger struct {
	Alias       string   `json:"alias,omitempty"`
	Middlewares []string `json:"middlewares,omitempty"`
}

// CronTrigger is a funcinst that will be scheduled by cron string
type CronTrigger struct {
	Cron string `json:"cron"`
	// Args is passed to function
	// Extra args will be appended to args
	// $time: RFC3339 formated time
	// $triggerName: name of trigger
	Args json.RawMessage `json:"args,omitempty"`
}

// HTTPTrigger is a funcinst that will react at HTTP requests
type HTTPTrigger struct {
	ContentType string `json:"contentType"`
}

// AsOwner returns *metav1.OwnerReference
func (t *Trigger) AsOwner() *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: APIVersion,
		Kind:       TriggerKind,
		Name:       t.Name,
		UID:        t.UID,
		Controller: &trueVar,
	}
}

// Ref returns *corev1.ObjectReference
func (t *Trigger) Ref() *corev1.ObjectReference {
	if t == nil {
		return nil
	}
	return &corev1.ObjectReference{
		APIVersion: APIVersion,
		Kind:       TriggerKind,
		Namespace:  t.Namespace,
		Name:       t.Name,
		UID:        t.UID,
	}
}
