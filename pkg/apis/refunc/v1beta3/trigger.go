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

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=triggers,singular=trigger,shortName=tr
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Trigger is a API object to represent a FUNCtion DEClaration
type Trigger struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec TriggerSpec `json:"spec"`
}

// +kubebuilder:object:root=true
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
	Event  *EventTrigger  `json:"event,omitempty"`
	Cron   *CronTrigger   `json:"cron,omitempty"`
	HTTP   *HTTPTrigger   `json:"http,omitempty"`
	Common *CommonTrigger `json:"common,omitempty"`
}

// EventTrigger is a basic trigger for a funcdef
type EventTrigger struct {
	Alias       string   `json:"alias,omitempty"`
	Middlewares []string `json:"middlewares,omitempty"`
}

// CronTrigger is a funcinst that will be scheduled by cron string
type CronTrigger struct {
	Cron string `json:"cron"`
	// time zoneinfo location name
	Location string `json:"location,omitempty"`
	// Args is passed to function
	// Extra args will be appended to args
	// $time: RFC3339 formated time
	// $triggerName: name of trigger
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Args json.RawMessage `json:"args,omitempty"`
	// If enable will save func exec's log or result to s3.
	SaveLog    bool `json:"saveLog,omitempty"`
	SaveResult bool `json:"saveResult,omitempty"`
}

// CommonTrigger is a placeholder trigger, for store the trigger config, and the trigger operator maybe not builtin.
type CommonTrigger struct {
	// +kubebuilder:validation:Schemaless
	// +kubebuilder:pruning:PreserveUnknownFields
	Args       json.RawMessage `json:"args,omitempty"`
	SaveLog    bool            `json:"saveLog,omitempty"`
	SaveResult bool            `json:"saveResult,omitempty"`
}

// HTTPTrigger is a funcinst that will react at HTTP requests
// https://docs.aws.amazon.com/lambda/latest/dg/lambda-urls.html
type HTTPTrigger struct {
	AuthType string          `json:"authType,omitempty"`
	Cors     HTTPTriggerCors `json:"cors,omitempty"`
}

type HTTPTriggerCors struct {
	AllowCredentials bool     `json:"allowCredentials,omitempty"`
	AllowHeaders     []string `json:"allowHeaders,omitempty"`
	AllowMethods     []string `json:"allowMethods,omitempty"`
	AllowOrigins     []string `json:"allowOrigins,omitempty"`
	ExposeHeaders    []string `json:"exposeHeaders,omitempty"`
	MaxAge           int      `json:"maxAge,omitempty"`
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
