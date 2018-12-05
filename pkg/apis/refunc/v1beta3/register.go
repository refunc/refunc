package v1beta3

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Third party resources group & version
const (
	// GroupName is the group name use in this package.
	GroupName  = "k8s.refunc.io"
	Version    = "v1beta3"
	APIVersion = GroupName + "/" + Version
)

// handle scheme
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme

	// SchemeGroupVersion is the group version used to register these objects.
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: Version}
)

// Resource takes an unqualified resource and returns a Group-qualified GroupResource.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// addKnownTypes adds the set of types defined in this package to the supplied scheme.
func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		new(Funcdef),
		new(FuncdefList),
		new(Xenv),
		new(XenvList),
		new(Trigger),
		new(TriggerList),
		new(Funcinst),
		new(FuncinstList),
	)
	metav1.AddToGroupVersion(scheme, SchemeGroupVersion)
	return nil
}
