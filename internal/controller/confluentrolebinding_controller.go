package controller

import (
	"context"
	"fmt"
	"time"

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

// ConfluentRoleBindingReconciler reconciles a ConfluentRoleBinding object
// against Confluent Cloud role bindings (authorization). The GitOps PR that
// creates/edits the CR is the approval gate.
type ConfluentRoleBindingReconciler struct {
	client.Client
	Config Config
}

// +kubebuilder:rbac:groups=confluentoauth.io,resources=confluentrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=confluentoauth.io,resources=confluentrolebindings/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=confluentoauth.io,resources=confluentrolebindings/finalizers,verbs=update
// +kubebuilder:rbac:groups=confluentoauth.io,resources=confluentidentitypools,verbs=get;list;watch

// Reconcile drives a ConfluentRoleBinding toward its desired Confluent state.
func (r *ConfluentRoleBindingReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var crb confluentv1alpha1.ConfluentRoleBinding
	if err := r.Get(ctx, req.NamespacedName, &crb); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !crb.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &crb)
	}

	if controllerutil.AddFinalizer(&crb, FinalizerName) {
		if err := r.Update(ctx, &crb); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Baseline for conflict-free status patches (see pool controller for rationale).
	orig := crb.DeepCopy()

	principal, requeue, err := r.resolvePrincipal(ctx, &crb)
	if err != nil {
		return ctrl.Result{}, err
	}
	if requeue {
		r.setCondition(&crb, "PrincipalResolved", metav1.ConditionFalse, "PoolNotReady", "referenced ConfluentIdentityPool has no poolId yet")
		if serr := r.Status().Patch(ctx, &crb, client.MergeFrom(orig)); serr != nil {
			return ctrl.Result{}, serr
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	crb.Status.Principal = principal
	r.setCondition(&crb, "PrincipalResolved", metav1.ConditionTrue, "Resolved", "principal resolved")

	desired := desiredBindingSet(crb.Spec.Bindings)
	applied := appliedBindingMap(crb.Status.AppliedBindings)

	next := make([]confluentv1alpha1.AppliedBinding, 0, len(desired))

	// Create any desired binding not yet applied.
	for key, rb := range desired {
		if existing, ok := applied[key]; ok {
			next = append(next, existing)
			continue
		}
		if r.Config.DryRun {
			logger.Info("[dry-run] would CREATE role binding", "principal", principal, "role", rb.RoleName, "crn", rb.CRNPattern)
			continue
		}
		created, cerr := r.Config.Confluent.CreateRoleBinding(ctx, confluent.RoleBinding{
			Principal:  principal,
			RoleName:   rb.RoleName,
			CRNPattern: rb.CRNPattern,
		})
		if cerr != nil {
			return r.degrade(ctx, &crb, orig, "CreateBindingFailed", cerr)
		}
		logger.Info("created role binding", "id", created.ID, "principal", principal, "role", rb.RoleName)
		next = append(next, confluentv1alpha1.AppliedBinding{
			ID:         created.ID,
			RoleName:   rb.RoleName,
			CRNPattern: rb.CRNPattern,
		})
	}

	// Prune any applied binding no longer desired.
	for key, ab := range applied {
		if _, ok := desired[key]; ok {
			continue
		}
		if r.Config.DryRun {
			logger.Info("[dry-run] would DELETE role binding", "id", ab.ID)
			continue
		}
		if derr := r.Config.Confluent.DeleteRoleBinding(ctx, ab.ID); derr != nil {
			return r.degrade(ctx, &crb, orig, "DeleteBindingFailed", derr)
		}
		logger.Info("pruned role binding", "id", ab.ID)
	}

	if r.Config.DryRun {
		r.setCondition(&crb, "BindingsReady", metav1.ConditionFalse, "DryRun", "would reconcile role bindings (dry-run: no write performed)")
		r.setCondition(&crb, "Degraded", metav1.ConditionFalse, "DryRun", "dry-run")
		if err := r.Status().Patch(ctx, &crb, client.MergeFrom(orig)); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: resyncInterval}, nil
	}

	crb.Status.AppliedBindings = next
	r.setCondition(&crb, "BindingsReady", metav1.ConditionTrue, "Reconciled", "role bindings reconciled")
	r.setCondition(&crb, "Degraded", metav1.ConditionFalse, "Reconciled", "no error")
	if err := r.Status().Patch(ctx, &crb, client.MergeFrom(orig)); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: resyncInterval}, nil
}

func (r *ConfluentRoleBindingReconciler) reconcileDelete(ctx context.Context, crb *confluentv1alpha1.ConfluentRoleBinding) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	if !controllerutil.ContainsFinalizer(crb, FinalizerName) {
		return ctrl.Result{}, nil
	}
	for _, ab := range crb.Status.AppliedBindings {
		if r.Config.DryRun {
			logger.Info("[dry-run] would DELETE role binding on teardown", "id", ab.ID)
			continue
		}
		if err := r.Config.Confluent.DeleteRoleBinding(ctx, ab.ID); err != nil {
			return ctrl.Result{}, fmt.Errorf("delete role binding %s: %w", ab.ID, err)
		}
		logger.Info("deleted role binding on teardown", "id", ab.ID)
	}
	controllerutil.RemoveFinalizer(crb, FinalizerName)
	return ctrl.Result{}, r.Update(ctx, crb)
}

// resolvePrincipal returns the Confluent principal for the binding. requeue is
// true when a referenced pool is not yet ready.
func (r *ConfluentRoleBindingReconciler) resolvePrincipal(ctx context.Context, crb *confluentv1alpha1.ConfluentRoleBinding) (principal string, requeue bool, err error) {
	if crb.Spec.Principal != "" {
		return crb.Spec.Principal, false, nil
	}
	if crb.Spec.PoolRef == nil {
		return "", false, fmt.Errorf("neither spec.principal nor spec.poolRef is set")
	}
	var pool confluentv1alpha1.ConfluentIdentityPool
	key := types.NamespacedName{Namespace: crb.Namespace, Name: crb.Spec.PoolRef.Name}
	if gerr := r.Get(ctx, key, &pool); gerr != nil {
		if apierrors.IsNotFound(gerr) {
			return "", true, nil
		}
		return "", false, gerr
	}
	if pool.Status.PoolID == "" {
		return "", true, nil
	}
	return "User:" + pool.Status.PoolID, false, nil
}

func (r *ConfluentRoleBindingReconciler) degrade(ctx context.Context, crb *confluentv1alpha1.ConfluentRoleBinding, orig *confluentv1alpha1.ConfluentRoleBinding, reason string, cause error) (ctrl.Result, error) {
	r.setCondition(crb, "Degraded", metav1.ConditionTrue, reason, cause.Error())
	r.setCondition(crb, "BindingsReady", metav1.ConditionFalse, reason, cause.Error())
	if serr := r.Status().Patch(ctx, crb, client.MergeFrom(orig)); serr != nil {
		return ctrl.Result{}, serr
	}
	return ctrl.Result{}, cause
}

func (r *ConfluentRoleBindingReconciler) setCondition(crb *confluentv1alpha1.ConfluentRoleBinding, condType string, status metav1.ConditionStatus, reason, msg string) {
	setStatusCondition(&crb.Status.Conditions, condType, status, reason, msg, crb.Generation)
}

// bindingKey uniquely identifies a role/CRN grant.
func bindingKey(roleName, crnPattern string) string {
	return roleName + "|" + crnPattern
}

func desiredBindingSet(bindings []confluentv1alpha1.RoleBinding) map[string]confluentv1alpha1.RoleBinding {
	m := make(map[string]confluentv1alpha1.RoleBinding, len(bindings))
	for _, b := range bindings {
		m[bindingKey(b.RoleName, b.CRNPattern)] = b
	}
	return m
}

func appliedBindingMap(applied []confluentv1alpha1.AppliedBinding) map[string]confluentv1alpha1.AppliedBinding {
	m := make(map[string]confluentv1alpha1.AppliedBinding, len(applied))
	for _, a := range applied {
		m[bindingKey(a.RoleName, a.CRNPattern)] = a
	}
	return m
}

// SetupWithManager registers the reconciler with the manager.
func (r *ConfluentRoleBindingReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&confluentv1alpha1.ConfluentRoleBinding{}).
		Complete(r)
}
