package controller

import (
	"fmt"
	"strings"

	"github.com/emanuelbesliu/confluent-identity-operator/internal/confluent"
)

// Config holds the operator-wide settings shared by both controllers, sourced
// from environment/Helm values.
type Config struct {
	// Confluent is the Confluent Cloud REST client (org-scoped credential).
	Confluent *confluent.Client

	// ProviderID is the Confluent identity provider (op-xxxx) all pools belong
	// to — one per Entra tenant/env (trust-layer output).
	ProviderID string

	// DefaultAudience is used when a ConfluentIdentityPool omits spec.audience.
	DefaultAudience string

	// ClusterName identifies the AKS cluster this operator instance serves. It
	// feeds deterministic pool naming and ownership scoping.
	ClusterName string

	// CRNScope optionally narrows role-binding lookups to an org/environment
	// CRN. May be empty.
	CRNScope string

	// DryRun, when true, makes every mutating Confluent Cloud call (create /
	// patch / delete of pools and role bindings) a no-op that is only logged.
	// Reads still happen. This is the primary safety gate for testing against a
	// real org.
	DryRun bool

	// OwnedPrefix, when non-empty, restricts the operator to only create, patch
	// or delete Confluent identity pools whose display name starts with this
	// prefix. Any resource outside the prefix is refused — a hard guard against
	// touching pools managed by other systems (e.g. Terraform-managed pools).
	OwnedPrefix string
}

// IsOwned reports whether a pool display name is within the operator's owned
// prefix. An empty OwnedPrefix owns everything (no restriction).
func (c Config) IsOwned(displayName string) bool {
	if c.OwnedPrefix == "" {
		return true
	}
	return strings.HasPrefix(displayName, c.OwnedPrefix)
}

// FinalizerName is the finalizer both controllers use to guarantee Confluent
// Cloud cleanup before the CR is removed.
const FinalizerName = "confluentoauth.io/finalizer"

// buildFilter returns the CEL filter that pins a pool to exactly one workload
// identity (audience + object id + client id). Kept well under Confluent's
// 300-char CEL limit.
func buildFilter(audience, principalID, clientID string) string {
	return fmt.Sprintf(`claims.aud=="%s" && claims.oid=="%s" && claims.azp=="%s"`,
		audience, principalID, clientID)
}
