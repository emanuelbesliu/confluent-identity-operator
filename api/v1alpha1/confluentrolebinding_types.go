package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PoolRef references a ConfluentIdentityPool in the same namespace. The
// operator resolves the bound principal as `User:<pool.status.poolId>` once the
// pool is ready.
type PoolRef struct {
	// Name of the ConfluentIdentityPool.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// RoleBinding is a single Confluent Cloud role binding: a role granted on a
// resource identified by a CRN pattern.
type RoleBinding struct {
	// RoleName is the Confluent Cloud role, e.g. DeveloperRead, DeveloperWrite,
	// ResourceOwner.
	// +kubebuilder:validation:MinLength=1
	RoleName string `json:"roleName"`

	// CRNPattern is the Confluent Resource Name pattern the role applies to,
	// e.g. `crn://confluent.cloud/.../topic=orders-*`.
	// +kubebuilder:validation:MinLength=1
	CRNPattern string `json:"crnPattern"`
}

// ConfluentRoleBindingSpec defines the desired authorization for a principal.
// It lives in the GitOps permission repo; the PR review IS the approval gate.
type ConfluentRoleBindingSpec struct {
	// PoolRef selects the identity pool whose principal is granted the bindings.
	// Exactly one of PoolRef or Principal must be set.
	// +optional
	PoolRef *PoolRef `json:"poolRef,omitempty"`

	// Principal is an explicit Confluent principal (e.g. `User:pool-abc123` or
	// `User:sa-xxxxx`). Use when the principal is not managed by a
	// ConfluentIdentityPool in this cluster. Exactly one of PoolRef or Principal
	// must be set.
	// +optional
	Principal string `json:"principal,omitempty"`

	// Bindings is the set of role/CRN grants for the principal.
	// +kubebuilder:validation:MinItems=1
	Bindings []RoleBinding `json:"bindings"`
}

// AppliedBinding records a role binding the operator created in Confluent Cloud.
type AppliedBinding struct {
	// ID is the Confluent Cloud role-binding id (rb-xxxx).
	ID string `json:"id"`
	// RoleName is the granted role.
	RoleName string `json:"roleName"`
	// CRNPattern is the resource pattern the role was granted on.
	CRNPattern string `json:"crnPattern"`
}

// ConfluentRoleBindingStatus defines the observed state.
type ConfluentRoleBindingStatus struct {
	// Principal is the resolved principal the bindings were applied to.
	// +optional
	Principal string `json:"principal,omitempty"`

	// AppliedBindings tracks the role bindings currently reconciled in Confluent
	// Cloud, so the operator can prune ones removed from spec.
	// +optional
	// +listType=map
	// +listMapKey=id
	AppliedBindings []AppliedBinding `json:"appliedBindings,omitempty"`

	// Conditions represent the latest available observations. Types:
	// PrincipalResolved, BindingsReady, Degraded.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=crb
// +kubebuilder:printcolumn:name="Principal",type=string,JSONPath=`.status.principal`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="BindingsReady")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ConfluentRoleBinding is the Schema for the confluentrolebindings API.
type ConfluentRoleBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfluentRoleBindingSpec   `json:"spec,omitempty"`
	Status ConfluentRoleBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfluentRoleBindingList contains a list of ConfluentRoleBinding.
type ConfluentRoleBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfluentRoleBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfluentRoleBinding{}, &ConfluentRoleBindingList{})
}
