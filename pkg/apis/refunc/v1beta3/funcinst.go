package v1beta3

import (
	"fmt"
	"path/filepath"
	reflect "reflect"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// CRD names for Funcdef
const (
	FuncinstKind       = "Funcinst"
	FuncinstPluralName = "funcinsts"
)

// static asserts
var (
	_ runtime.Object            = (*Funcinst)(nil)
	_ metav1.ObjectMetaAccessor = (*Funcinst)(nil)

	_ runtime.Object          = (*FuncinstList)(nil)
	_ metav1.ListMetaAccessor = (*FuncinstList)(nil)
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=funcinsts,singular=funcinst,shortName=fni
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Funcinst is a API object to represent a FUNCtion INSTance
type Funcinst struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   FuncinstSpec   `json:"spec"`
	Status FuncinstStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FuncinstList is a API object to represent a list of Refuncs
type FuncinstList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Funcinst `json:"items"`
}

// FuncinstSpec is the specification that describes a funcinst for refunc
type FuncinstSpec struct {
	FuncdefRef *corev1.ObjectReference `json:"funcdefRef,omitempty"`
	TriggerRef *corev1.ObjectReference `json:"triggerRef,omitempty"`

	Runtime RuntimeContext `json:"runtime"`
}

// FuncinstStatus is the running status for a refunc
type FuncinstStatus struct {
	// Current service state of funcinst.
	Conditions []FuncinstCondition `json:"conditions,omitempty"`
	// number of active instances
	Active int `json:"active,omitempty"`
}

// FuncinstConditionType is label to indicates current state for a func
type FuncinstConditionType string

// Different phases during life time of a funcinst
const (
	FuncinstInactive FuncinstConditionType = "Inactive" // funcinst cannot accept new events
	FuncinstPending  FuncinstConditionType = "Pending"  // waiting for a valid xenv is ready
	FuncinstActive   FuncinstConditionType = "Active"   // can be invoked
)

// FuncinstCondition contains details for the current condition of this funcinst.
type FuncinstCondition struct {
	// Type of cluster condition.
	Type FuncinstConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime string `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}

// RuntimeContext is information about runtime shard by pods accross funcinst
type RuntimeContext struct {
	Credentials Credentials `json:"credentials"`
	Permissions Permissions `json:"permissions"`
}

// Credentials provides runtime credentials for funcinst
type Credentials struct {
	AccessKey string `json:"accessKey,omitempty"`
	SecretKey string `json:"secretKey,omitempty"`
	Token     string `json:"token,omitempty"`
}

// Permissions provides runtime permissions for funcinst
type Permissions struct {
	Scope     string   `json:"scope,omitempty"`
	Publish   []string `json:"publish,omitempty"`
	Subscribe []string `json:"subscribe,omitempty"`
}

// AsOwner returns *metav1.OwnerReference
func (t *Funcinst) AsOwner() *metav1.OwnerReference {
	return &metav1.OwnerReference{
		APIVersion: APIVersion,
		Kind:       FuncinstKind,
		Name:       t.Name,
		UID:        t.UID,
		Controller: &trueVar,
	}
}

// Ref returns *corev1.ObjectReference
func (t *Funcinst) Ref() *corev1.ObjectReference {
	if t == nil {
		return nil
	}
	return &corev1.ObjectReference{
		APIVersion: APIVersion,
		Kind:       FuncinstKind,
		Namespace:  t.Namespace,
		Name:       t.Name,
		UID:        t.UID,
	}
}

// OnlyLastActivityChanged checks two funcinst returns true if only LastActivity is changed
func OnlyLastActivityChanged(left, right *Funcinst) bool {
	l, r := left.DeepCopy(), right.DeepCopy()
	l.TypeMeta = r.TypeMeta
	l.ObjectMeta = r.ObjectMeta
	// l.Spec.LastActivity = r.Spec.LastActivity
	l.Status.SetCondition(*r.Status.ActiveCondition())
	return reflect.DeepEqual(l, r)
}

// NewDefaultPermissions returns default permissions for given inst
func NewDefaultPermissions(fni *Funcinst, scopeRoot string) Permissions {
	return Permissions{
		Scope: filepath.Join(scopeRoot, fni.Spec.FuncdefRef.Namespace, fni.Spec.FuncdefRef.Name, "data") + "/",
		Publish: []string{
			// request endpoint
			"refunc.*.*",
			"refunc.*.*._meta",
			// reply
			"_INBOX.*",   // old style
			"_INBOX.*.*", // new style
			// logs forwarding endpoint to client
			"_refunc.forwardlogs.*",
			fni.EventsPubEndpoint(),
			fni.LoggingEndpoint(),
			fni.CryingEndpoint(),
			fni.TappingEndpoint(),
		},
		Subscribe: []string{
			// public
			"_INBOX.*",   // old style
			"_INBOX.*.*", // new style
			// internal
			fni.EventsSubEndpoint(),
			fni.ServiceEndpoint(),
			fni.CryServiceEndpoint(),
		},
	}
}

// EventsPubEndpoint is endpoint for publishing events
func (t *Funcinst) EventsPubEndpoint() string {
	return fmt.Sprintf("refunc.%s.%s.events.>", t.Spec.FuncdefRef.Namespace, t.Spec.FuncdefRef.Name)
}

// LoggingEndpoint is endpoint for logging
func (t *Funcinst) LoggingEndpoint() string {
	return fmt.Sprintf("refunc.%s.%s.logs.%s", t.Spec.FuncdefRef.Namespace, t.Spec.FuncdefRef.Name, t.Name)
}

// CryingEndpoint is endpoint to signal birth of a inst
func (t *Funcinst) CryingEndpoint() string {
	return fmt.Sprintf("_refunc._cry_.%s/%s", t.Namespace, t.Name)
}

// TappingEndpoint is endpoint for tapping
func (t *Funcinst) TappingEndpoint() string {
	return fmt.Sprintf("_refunc._tap_.%s/%s", t.Namespace, t.Name)
}

// EventsSubEndpoint is endpoint for subscribing events within same ns
func (t *Funcinst) EventsSubEndpoint() string {
	return fmt.Sprintf("refunc.%s.*.events.>", t.Spec.FuncdefRef.Namespace)
}

// ServiceEndpoint is endpoint for inst to listen at in order to proivde services
func (t Funcinst) ServiceEndpoint() string {
	return fmt.Sprintf("_refunc._insts_.%s.%s", t.Spec.FuncdefRef.Namespace, t.Name)
}

// CryServiceEndpoint is endpoint to poke a inst to cry
func (t *Funcinst) CryServiceEndpoint() string {
	return t.ServiceEndpoint() + "._cry_"
}

// Touch updates LastUpdateTime of active conidtion
func (ts *FuncinstStatus) Touch() *FuncinstStatus {
	ts.ActiveCondition().LastUpdateTime = time.Now().Format(time.RFC3339)
	return ts
}

// LastActivity returns last observed activity time
func (ts *FuncinstStatus) LastActivity() time.Time {
	if tm := ts.ActiveCondition().LastUpdateTime; tm != "" {
		t, _ := time.Parse(time.RFC3339, tm)
		return t
	}
	return time.Time{}
}

// ActiveCondition gets or creates a active condition
func (ts *FuncinstStatus) ActiveCondition() *FuncinstCondition {
	_, active := getFuncinstCondition(ts, FuncinstActive)
	if active == nil {
		active = newFuncinstCondition(FuncinstActive, corev1.ConditionFalse, "Created", "Created by function access")
		active.LastUpdateTime = ""
		if !ts.IsInactiveCondition() {
			ts.SetCondition(*active)
			_, active = getFuncinstCondition(ts, FuncinstActive)
		}
	}
	return active
}

// IsActiveCondition returns true if current funcinst is marked as active
func (ts *FuncinstStatus) IsActiveCondition() bool {
	if _, active := getFuncinstCondition(ts, FuncinstActive); active != nil {
		return active.Status == corev1.ConditionTrue
	}
	return false
}

// IsInactiveCondition returns true if current funcinst is marked as inactive
func (ts *FuncinstStatus) IsInactiveCondition() bool {
	if _, inactive := getFuncinstCondition(ts, FuncinstInactive); inactive != nil {
		return inactive.Status == corev1.ConditionTrue
	}
	return false
}

// SetActiveCondition turns this funcinst into active
func (ts *FuncinstStatus) SetActiveCondition(reason, message string) *FuncinstStatus {
	if ts.IsInactiveCondition() {
		return ts
	}
	active := ts.ActiveCondition()
	active.Status = corev1.ConditionTrue
	if active.Reason != reason && active.Message != message {
		active.LastTransitionTime = time.Now().Format(time.RFC3339)
		active.Reason = reason
		active.Message = message
	}
	return ts.ClearCondition(FuncinstPending).SetCondition(*active)
}

// SetInactiveCondition turns this funcinst into inactive
func (ts *FuncinstStatus) SetInactiveCondition(reason, message string) *FuncinstStatus {
	return ts.Deactive(
		"FuninstIsInactive",
		"Funinst was put into inactive",
	).SetCondition(
		*newFuncinstCondition(FuncinstInactive, corev1.ConditionTrue, reason, message),
	)
}

// SetPendingCondition turns this funcinst into pending
func (ts *FuncinstStatus) SetPendingCondition(reason, message string) *FuncinstStatus {
	return ts.Deactive(
		"FuninstIsPending",
		"Funinst was put into pendding",
	).SetCondition(
		*newFuncinstCondition(FuncinstPending, corev1.ConditionTrue, reason, message),
	)
}

// Deactive turns active condition(if any) into False, returns *FuncinstStatus for chaining
func (ts *FuncinstStatus) Deactive(reason, message string) *FuncinstStatus {
	if _, active := getFuncinstCondition(ts, FuncinstActive); active != nil && active.Status == corev1.ConditionTrue {
		active.Status = corev1.ConditionFalse
		active.LastTransitionTime = time.Now().Format(time.RFC3339)
		active.Reason = reason
		active.Message = message
	}
	return ts
}

// SetCondition sets or inserts funcinst condition
func (ts *FuncinstStatus) SetCondition(c FuncinstCondition) *FuncinstStatus {
	pos, cp := getFuncinstCondition(ts, c.Type)
	if cp != nil && cp.Status == c.Status && cp.Reason == c.Reason && cp.Message == c.Message {
		return ts
	}

	if cp != nil {
		ts.Conditions[pos] = c
	} else {
		ts.Conditions = append(ts.Conditions, c)
	}
	return ts
}

// ClearCondition removes confition of given type from conditions
func (ts *FuncinstStatus) ClearCondition(t FuncinstConditionType) *FuncinstStatus {
	pos, _ := getFuncinstCondition(ts, t)
	if pos == -1 {
		return ts
	}
	ts.Conditions = append(ts.Conditions[:pos], ts.Conditions[pos+1:]...)
	return ts
}

func getFuncinstCondition(status *FuncinstStatus, t FuncinstConditionType) (int, *FuncinstCondition) {
	for i := range status.Conditions {
		if t == status.Conditions[i].Type {
			return i, &status.Conditions[i]
		}
	}
	return -1, nil
}

func newFuncinstCondition(condType FuncinstConditionType, status corev1.ConditionStatus, reason, message string) *FuncinstCondition {
	now := time.Now().Format(time.RFC3339)
	return &FuncinstCondition{
		Type:               condType,
		Status:             status,
		LastUpdateTime:     now,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}
}
