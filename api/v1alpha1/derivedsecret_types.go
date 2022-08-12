/*
Copyright 2022 Andrew Melnick

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DerivedSecretSpec defines the desired state of DerivedSecret
type DerivedSecretSpec struct {
	// References is a list of ConfigMaps or Secrets that can be referenced in the data or stringData templates
	References []SensitiveReference `json:"references"`
	// ServiceAccountName is the name of a ServiceAccount in the same Namespace as the DerivedSecret that will be used to create the derived Secret
	// Required if targetNamespace is set, and not the same as the current namespace
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// TargetType is the "type" field of the derived Secret. Same default as a Secret
	// +optional
	TargetType corev1.SecretType `json:"targetType"`
	// TargetName is the name of the Secret to create. Defaults to the same as the DerivedSecret
	// +optional
	TargetName string `json:"targetName,omitempty"`
	// TargetNamespace is the name of the Secret to create. Defaults to the same as the DerivedSecret
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// Data is a map of keys to values that should produce base64-encoded binary data (e.g. with b64enc) to include in the Secret's data
	// +optional
	// Data FieldSet `json:"data,omitempty"` // controller-tools doesn't work
	Data map[string]BinaryTarget `json:"data,omitempty"`
	// StringData is a set of map of keys to templates that should produce string data to include in the Secret's stringData
	// +optional
	// StringData FieldSet `json:"stringData,omitempty"` // controller-tools doesn't work
	StringData map[string]StringTarget `json:"stringData,omitempty"`
	// Prefab is a set of common options to use instead of data/stringData
	// +optional
	Prefab *Prefabs `json:"prefab,omityempty"`
	// TODO: optional cleanup field
}

// DerivedSecretStatus defines the observed state of DerivedSecret
type DerivedSecretStatus struct {
	// SecretName is the name of the secret that was generated, if any
	// +optional
	SecretName string `json:"secretName,omitempty"`
	// SecretNamespace is the namespace of the secret that was generated, if any
	// +optional
	SecretNamespace string `json:"secretNamespace,omitempty"`
	// Error is the error message from the last sync attempt, if any
	// +optional
	Error string `json:"error,omitempty"`
	// LastSync is the time when the secret was last generated
	// +optional
	LastSync *metav1.Time `json:"lastSync,omitempty"`
	// LastSyncAttempt is the time when the secret was last attmpted to be generated
	// +optional
	LastSyncAttempt *metav1.Time `json:"lastSyncAttempt,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DerivedSecret is the Schema for the derivedsecrets API
type DerivedSecret struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DerivedSecretSpec   `json:"spec,omitempty"`
	Status DerivedSecretStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DerivedSecretList contains a list of DerivedSecret
type DerivedSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DerivedSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DerivedSecret{}, &DerivedSecretList{})
}
