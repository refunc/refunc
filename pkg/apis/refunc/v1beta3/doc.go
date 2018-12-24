/*
Package v1beta3 is list of k8s objects for refunc
*/
// +k8s:deepcopy-gen=package,register
// +groupName=refunc.refunc.io
package v1beta3

import (
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// well known labels
const (
	LabelResType = "refunc.io/res"
	LabelName    = "refunc.io/name"
	LabelHash    = "refunc.io/hash"
	LabelUID     = "refunc.io/uid"

	LabelLambdaVersion = "lambda.refunc.io/version"
	LabelLambdaName    = "lambda.refunc.io/name"

	LabelRunner        = "refunc.io/runner"
	LabelRunnerIsReady = "refunc.io/runner-ready"

	LabelExecutor        = "refunc.io/executor"
	LabelExecutorIsReady = "refunc.io/executor-is-ready"

	// Label to select operator
	LabelTrigger     = "refunc.io/trigger"
	LabelTriggerType = "refunc.io/trigger-type"

	// Annotations to enable API compatible features
	AnnotationRPCVer = "refunc.io/rpc-version"
)

var trueVar = true

// CRDs is collections of ThirdPartyResources
var CRDs = []struct {
	Name string
	CRD  *apiextensionsv1beta1.CustomResourceDefinition
}{
	// Funcdef
	{
		FuncdefPluralName,
		&apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: FuncdefPluralName + "." + GroupName,
			},
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:   SchemeGroupVersion.Group,
				Version: SchemeGroupVersion.Version,
				Scope:   apiextensionsv1beta1.NamespaceScoped,
				Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
					Plural:     FuncdefPluralName,
					Kind:       FuncdefKind,
					ShortNames: []string{"fnd"},
				},
			},
		},
	},
	// Xenv
	{
		XenvPluralName,
		&apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: XenvPluralName + "." + GroupName,
			},
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:   SchemeGroupVersion.Group,
				Version: SchemeGroupVersion.Version,
				Scope:   apiextensionsv1beta1.NamespaceScoped,
				Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
					Plural:     XenvPluralName,
					Kind:       XenvKind,
					ShortNames: []string{"xe"},
				},
			},
		},
	},
	// Trigger
	{
		TriggerPluralName,
		&apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: TriggerPluralName + "." + GroupName,
			},
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:   SchemeGroupVersion.Group,
				Version: SchemeGroupVersion.Version,
				Scope:   apiextensionsv1beta1.NamespaceScoped,
				Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
					Plural:     TriggerPluralName,
					Kind:       TriggerKind,
					ShortNames: []string{"tr"},
				},
			},
		},
	},
	// Funcinst
	{
		FuncinstPluralName,
		&apiextensionsv1beta1.CustomResourceDefinition{
			ObjectMeta: metav1.ObjectMeta{
				Name: FuncinstPluralName + "." + GroupName,
			},
			Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
				Group:   SchemeGroupVersion.Group,
				Version: SchemeGroupVersion.Version,
				Scope:   apiextensionsv1beta1.NamespaceScoped,
				Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
					Plural:     FuncinstPluralName,
					Kind:       FuncinstKind,
					ShortNames: []string{"fni"},
				},
			},
		},
	},
}
