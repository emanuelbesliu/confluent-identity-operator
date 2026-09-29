package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IdentityConfigRef points at the ConfigMap (exported by ASO
// operatorSpec.configMaps) that holds the UAMI's clientId (azp) and
// principalId (oid). Mirrors the existing `principalIdFromConfig` idiom used by
// the ASO RoleAssignment CRs, so the same ConfigMap is reused.
type IdentityConfigRef struct {
	// Name of the ConfigMap holding the UAMI identity attributes.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// ClientIDKey is the ConfigMap key holding the UAMI client id (azp claim).
	// +kubebuilder:default=clientId
	// +optional
	ClientIDKey string `json:"clientIdKey,omitempty"`

	// PrincipalIDKey is the ConfigMap key holding the UAMI principal/object id
	// (oid claim).
	// +kubebuilder:default=principalId
	// +optional
	PrincipalIDKey string `json:"principalIdKey,omitempty"`
}

// ConfluentIdentityPoolSpec defines the desired state of a Confluent Cloud
// identity pool for a single AKS workload. A pool created by this operator has
// ZERO role bindings — it grants authentication only; every topic operation is
// 403 until a ConfluentRoleBinding grants access.
type ConfluentIdentityPoolSpec struct {
	// IdentityConfigRef is where to read the workload's azp (clientId) and oid
	// (principalId) that pin the pool's CEL filter.
	IdentityConfigRef IdentityConfigRef `json:"identityConfigRef"`

	// Audience is the shared audience app registration client-id that all pool
	// filters pin as `claims.aud`. Sourced from the trust-layer Terraform
	// output; overridable per-CR.
	// +kubebuilder:validation:MinLength=1
	Audience string `json:"audience"`

	// DisplayName is the deterministic Confluent pool display name
	// (`<cluster>-<ns>-<app>`). Used as the idempotency key when looking the
	// pool up in Confluent Cloud.
	// +kubebuilder:validation:MinLength=1
	DisplayName string `json:"displayName"`

	// Description is an optional human-readable description stored on the pool.
	// +optional
	Description string `json:"description,omitempty"`
}

// ConfluentIdentityPoolStatus defines the observed state.
type ConfluentIdentityPoolStatus struct {
	// PoolID is the Confluent Cloud identity pool id (pool-xxxx).
	// +optional
	PoolID string `json:"poolId,omitempty"`

	// ProviderID is the identity provider the pool belongs to (op-xxxx).
	// +optional
	ProviderID string `json:"providerId,omitempty"`

	// AppliedFilter is the CEL filter currently set on the pool.
	// +optional
	AppliedFilter string `json:"appliedFilter,omitempty"`

	// ObservedClientID is the azp value last read from the identity ConfigMap.
	// +optional
	ObservedClientID string `json:"observedClientId,omitempty"`

	// ObservedPrincipalID is the oid value last read from the identity ConfigMap.
	// +optional
	ObservedPrincipalID string `json:"observedPrincipalId,omitempty"`

	// Conditions represent the latest available observations. Types:
	// IdentityResolved, PoolReady, Degraded.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=cip
// +kubebuilder:printcolumn:name="Pool",type=string,JSONPath=`.status.poolId`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="PoolReady")].status`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// ConfluentIdentityPool is the Schema for the confluentidentitypools API.
type ConfluentIdentityPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ConfluentIdentityPoolSpec   `json:"spec,omitempty"`
	Status ConfluentIdentityPoolStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ConfluentIdentityPoolList contains a list of ConfluentIdentityPool.
type ConfluentIdentityPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ConfluentIdentityPool `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ConfluentIdentityPool{}, &ConfluentIdentityPoolList{})
}
