package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	confluentv1alpha1 "github.com/emanuelbesliu/confluent-identity-operator/api/v1alpha1"
	"github.com/emanuelbesliu/confluent-identity-operator/internal/confluent"
)

const identityClaim = "claims.sub"

// resyncInterval drives periodic drift detection (e.g. UAMI recreated).
const resyncInterval = 10 * time.Minute

// ConfluentIdentityPoolReconciler reconciles a ConfluentIdentityPool object
// against a Confluent Cloud identity pool (authentication only).
type ConfluentIdentityPoolReconciler struct {
	client.Client
	Config Config
}

// +kubebuilder:rbac:groups=confluentoauth.io,resources=confluentidentitypools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=confluentoauth.io,resources=confluentidentitypools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=confluentoauth.io,resources=confluentidentitypools/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

// Reconcile drives a ConfluentIdentityPool toward its desired Confluent state.
func (r *ConfluentIdentityPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pool confluentv1alpha1.ConfluentIdentityPool
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !pool.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &pool)
	}

	if controllerutil.AddFinalizer(&pool, FinalizerName) {
		if err := r.Update(ctx, &pool); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Baseline for status patches. Patching status with MergeFrom avoids the
	// resourceVersion precondition that r.Status().Update() carries, so racing
	// reconciles (e.g. the finalizer write above plus the create event) can no
	// longer collide with "the object has been modified" conflicts.
	orig := pool.DeepCopy()

	clientID, principalID, err := r.readIdentityConfig(ctx, &pool)
	if err != nil {
		if apierrors.IsNotFound(err) || isMissingKey(err) {
			logger.Info("identity config not ready, requeueing", "reason", err.Error())
			r.setCondition(&pool, "IdentityResolved", metav1.ConditionFalse, "ConfigNotReady", err.Error())
			if serr := r.Status().Patch(ctx, &pool, client.MergeFrom(orig)); serr != nil {
				return ctrl.Result{}, serr
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		return ctrl.Result{}, err
	}
	r.setCondition(&pool, "IdentityResolved", metav1.ConditionTrue, "Resolved", "clientId and principalId resolved from ConfigMap")

	audience := pool.Spec.Audience
	if audience == "" {
		audience = r.Config.DefaultAudience
	}
	desiredFilter := buildFilter(audience, principalID, clientID)

	existing, err := r.Config.Confluent.FindIdentityPoolByDisplayName(ctx, r.Config.ProviderID, pool.Spec.DisplayName)
	if err != nil {
		return r.degrade(ctx, &pool, orig, "FindFailed", err)
	}

	// Ownership guard: never create/patch a pool outside the owned prefix.
	if !r.Config.IsOwned(pool.Spec.DisplayName) {
		err := fmt.Errorf("refusing to manage pool %q: outside owned prefix %q", pool.Spec.DisplayName, r.Config.OwnedPrefix)
		return r.degrade(ctx, &pool, orig, "NotOwned", err)
	}

	var result *confluent.IdentityPool
	switch {
	case existing == nil:
		if r.Config.DryRun {
			logger.Info("[dry-run] would CREATE identity pool", "displayName", pool.Spec.DisplayName, "filter", desiredFilter, "identityClaim", identityClaim)
			r.setCondition(&pool, "PoolReady", metav1.ConditionFalse, "DryRun", "would create pool (dry-run: no write performed)")
			r.setCondition(&pool, "Degraded", metav1.ConditionFalse, "DryRun", "dry-run")
			if err := r.Status().Patch(ctx, &pool, client.MergeFrom(orig)); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: resyncInterval}, nil
		}
		created, cerr := r.Config.Confluent.CreateIdentityPool(ctx, r.Config.ProviderID, confluent.IdentityPool{
			DisplayName:   pool.Spec.DisplayName,
			Description:   pool.Spec.Description,
			IdentityClaim: identityClaim,
			Filter:        desiredFilter,
		})
		if cerr != nil {
			return r.degrade(ctx, &pool, orig, "CreateFailed", cerr)
		}
		logger.Info("created identity pool", "poolId", created.ID, "displayName", pool.Spec.DisplayName)
		result = created
	case existing.Filter != desiredFilter:
		if r.Config.DryRun {
			logger.Info("[dry-run] would PATCH identity pool filter", "poolId", existing.ID, "from", existing.Filter, "to", desiredFilter)
			pool.Status.PoolID = existing.ID
			pool.Status.ProviderID = r.Config.ProviderID
			r.setCondition(&pool, "PoolReady", metav1.ConditionFalse, "DryRun", "would patch pool filter (dry-run: no write performed)")
			r.setCondition(&pool, "Degraded", metav1.ConditionFalse, "DryRun", "dry-run")
			if err := r.Status().Patch(ctx, &pool, client.MergeFrom(orig)); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: resyncInterval}, nil
		}
		updated, uerr := r.Config.Confluent.UpdateIdentityPool(ctx, r.Config.ProviderID, existing.ID, confluent.IdentityPool{
			DisplayName:   pool.Spec.DisplayName,
			Description:   pool.Spec.Description,
			IdentityClaim: identityClaim,
			Filter:        desiredFilter,
		})
		if uerr != nil {
			return r.degrade(ctx, &pool, orig, "UpdateFailed", uerr)
		}
		logger.Info("patched drifted identity pool filter", "poolId", updated.ID)
		result = updated
	default:
		result = existing
	}

	pool.Status.PoolID = result.ID
	pool.Status.ProviderID = r.Config.ProviderID
	pool.Status.AppliedFilter = desiredFilter
	pool.Status.ObservedClientID = clientID
	pool.Status.ObservedPrincipalID = principalID
	r.setCondition(&pool, "PoolReady", metav1.ConditionTrue, "Reconciled", "identity pool reconciled")
	r.setCondition(&pool, "Degraded", metav1.ConditionFalse, "Reconciled", "no error")
	if err := r.Status().Patch(ctx, &pool, client.MergeFrom(orig)); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: resyncInterval}, nil
}

func (r *ConfluentIdentityPoolReconciler) reconcileDelete(ctx context.Context, pool *confluentv1alpha1.ConfluentIdentityPool) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(pool, FinalizerName) {
		return ctrl.Result{}, nil
	}
	if pool.Status.PoolID != "" {
		switch {
		case r.Config.DryRun:
			logger.Info("[dry-run] would DELETE identity pool", "poolId", pool.Status.PoolID, "displayName", pool.Spec.DisplayName)
		case !r.Config.IsOwned(pool.Spec.DisplayName):
			logger.Info("refusing to delete pool outside owned prefix; leaving it untouched",
				"poolId", pool.Status.PoolID, "displayName", pool.Spec.DisplayName, "ownedPrefix", r.Config.OwnedPrefix)
		default:
			if err := r.Config.Confluent.DeleteIdentityPool(ctx, r.Config.ProviderID, pool.Status.PoolID); err != nil {
				return ctrl.Result{}, fmt.Errorf("delete pool %s: %w", pool.Status.PoolID, err)
			}
			logger.Info("deleted identity pool", "poolId", pool.Status.PoolID)
		}
	}
	controllerutil.RemoveFinalizer(pool, FinalizerName)
	return ctrl.Result{}, r.Update(ctx, pool)
}

// missingKeyError signals the identity ConfigMap exists but lacks a required key.
type missingKeyError struct{ key string }

func (e *missingKeyError) Error() string { return fmt.Sprintf("configmap missing key %q", e.key) }

func isMissingKey(err error) bool {
	_, ok := err.(*missingKeyError)
	return ok
}

func (r *ConfluentIdentityPoolReconciler) readIdentityConfig(ctx context.Context, pool *confluentv1alpha1.ConfluentIdentityPool) (clientID, principalID string, err error) {
	ref := pool.Spec.IdentityConfigRef
	clientKey := ref.ClientIDKey
	if clientKey == "" {
		clientKey = "clientId"
	}
	principalKey := ref.PrincipalIDKey
	if principalKey == "" {
		principalKey = "principalId"
	}

	var cm corev1.ConfigMap
	if err := r.Get(ctx, types.NamespacedName{Namespace: pool.Namespace, Name: ref.Name}, &cm); err != nil {
		return "", "", err
	}
	clientID = cm.Data[clientKey]
	principalID = cm.Data[principalKey]
	if clientID == "" {
		return "", "", &missingKeyError{key: clientKey}
	}
	if principalID == "" {
		return "", "", &missingKeyError{key: principalKey}
	}
	return clientID, principalID, nil
}

func (r *ConfluentIdentityPoolReconciler) degrade(ctx context.Context, pool *confluentv1alpha1.ConfluentIdentityPool, orig *confluentv1alpha1.ConfluentIdentityPool, reason string, cause error) (ctrl.Result, error) {
	r.setCondition(pool, "Degraded", metav1.ConditionTrue, reason, cause.Error())
	r.setCondition(pool, "PoolReady", metav1.ConditionFalse, reason, cause.Error())
	if serr := r.Status().Patch(ctx, pool, client.MergeFrom(orig)); serr != nil {
		return ctrl.Result{}, serr
	}
	return ctrl.Result{}, cause
}

func (r *ConfluentIdentityPoolReconciler) setCondition(pool *confluentv1alpha1.ConfluentIdentityPool, condType string, status metav1.ConditionStatus, reason, msg string) {
	setStatusCondition(&pool.Status.Conditions, condType, status, reason, msg, pool.Generation)
}

// SetupWithManager registers the reconciler with the manager.
func (r *ConfluentIdentityPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&confluentv1alpha1.ConfluentIdentityPool{}).
		Complete(r)
}
