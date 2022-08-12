package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	client "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DerivedFromNameLabel      = "secrets-operator.meln5674.github.com/derived-from.name"
	DerivedFromNamespaceLabel = "secrets-operator.meln5674.github.com/derived-from.namespace"
	DerivedFromGroupLabel     = "secrets-operator.meln5674.github.com/derived-from.group"
	DerivedFromKindLabel      = "secrets-operator.meln5674.github.com/derived-from.kind"
	DerivedFromVersionLabel   = "secrets-operator.meln5674.github.com/derived-from.version"
)

func DerivedFromLabelValues(obj client.Object) map[string]string {
	gvk := obj.GetObjectKind().GroupVersionKind()
	return map[string]string{
		DerivedFromNameLabel:      obj.GetName(),
		DerivedFromNamespaceLabel: obj.GetNamespace(),
		DerivedFromGroupLabel:     gvk.Group,
		DerivedFromVersionLabel:   gvk.Version,
		DerivedFromKindLabel:      gvk.Kind,
	}
}

// ReferenceName is the name of a referenced ConfigMap or Secret
// type ReferenceName = string

// ReferenceKey is a key in a referenced ConfigMap or Secret
// type ReferenceKey = string

// Template is a golang text/template template string
// type Template = string

// FieldSet is a list map of keys to golang text/template template strings
// type FieldSet = map[ReferenceKey]Template

// Reference binds a ConfigMap to a name that can be referenced in a template
type Reference struct {
	// Name is the name to reference this ConfigMap in a template
	// Name ReferenceName `json:"name"` // controller-tools doesn't work
	Name string `json:"name"`
	// ConfigMapRef specifies the ConfigMap to use
	ConfigMapRef corev1.ConfigMapEnvSource `json:"configMapRef"`
}

// AsSensitiveReference converts a non-sensitive (ConfigMap) Reference to one that could possibly contain a Secret instead
func (r *Reference) AsSensitiveReference() SensitiveReference {
	return SensitiveReference{
		Name:         r.Name,
		ConfigMapRef: &r.ConfigMapRef,
	}
}

// SensitiveReference binds a Secret or ConfigMap to a name that can be referenced in a template
type SensitiveReference struct {
	// Name is the name to reference this ConfigMap/Secret
	// Name ReferenceName `json:"name"` // controller-tools doesn't work
	Name string `json:"name"`
	// ConfigMapRef specifies a ConfigMap to use
	// +optional
	ConfigMapRef *corev1.ConfigMapEnvSource `json:"configMapRef,omitEmpty"`
	// SecretRef specifies a Secret to use
	// +optional
	SecretRef *corev1.SecretEnvSource `json:"secretRef,omityEmpty"`
}

// ReferenceSubset refers to a subset of keys in a Reference
type ReferenceSubset struct {
	// Name is the name of the Reference in question
	// Name ReferenceName `json:"name"` // controller-tools doesn't work
	Name string `json:"name"`
	// Keys is the list of keys in question
	// +optional
	// Keys []ReferenceKey `json:"keys,omitempty"` // controller-tools doesn't work
	Keys []string `json:"keys,omitempty"`
	// AllKeys indicates all keys in the Reference should be considered
	// +optional
	AllKeys *bool `json:"allKeys,omityempty"`
}

const (
	DefaultTargetOverwrite = true
)

type TargetBase struct {
	// Template is a golang text/template template to evaluate using References. If this is in a Secret.data or ConfigMap.binaryData, this is expected to produce base64-encoded data
	// +optional
	Template *string `json:"template,omitempty"`
	// Overwrite indicates that the operator should overwrite any value with the same same when updating the derived Secret or ConfigMap, if false, it will be left alone
	// +optional
	Overwrite *bool `json:"overwrite"`
}

// Target specifies a target field in a Secret.stringData or ConfigMap.data
type StringTarget struct {
	TargetBase `json:",inline"`
	// Literal is a literal string to set. If this is in a Secret.data or ConfigMap.binaryData, this is expected to be base64-encoded
	// +optional
	Literal *string `json:"literal,omitempty"`
}

// Target specifies a target field in a Secret.stringData or ConfigMap.data
type BinaryTarget struct {
	TargetBase `json:",inline"`
	// Literal is a literal string to set. If this is in a Secret.data or ConfigMap.binaryData, this is expected to be base64-encoded
	// +optional
	Literal []byte `json:"literal,omitempty"`
}

// Prefabs specifies common use cases to use instead of manually defining data/stringData/binaryData
type Prefabs struct {
	// CopyAll indicates that all keys from all references should be copied verbatim, and produce an error if any keys overlap
	// +optional
	CopyAll *bool `json:"copyAll,omitempty"`
	// CopyIncluding indicates that just the specified keys in the specified references should be copied verbatim, and produce an error if any keys overlap
	// +optional
	CopyIncluding []ReferenceSubset `json:"copyIncluding,omitempty"`
	// CopyExcluding indicates that all but the specified keys in the specified references should be copied verbatim, and produce an error if any keys overlap
	// +optional
	CopyExcluding []ReferenceSubset `json:"copyExcluding,omitempty"`
}
