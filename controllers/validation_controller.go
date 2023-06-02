// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	sourcebeta2 "github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/open-component-model/mpas-product-controller/pkg/validators"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	gitv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	gitmpasv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
)

// ValidationReconciler reconciles a Validation object
type ValidationReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	MpasSystemNamespace string
	Validator           validators.Validator
}

// SetupWithManager sets up the controller with the Manager.
func (r *ValidationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &sourcebeta2.GitRepository{}, controllerMetadataKey, func(rawObj client.Object) []string {
		repository := rawObj.(*sourcebeta2.GitRepository)
		owner := metav1.GetControllerOf(repository)
		if owner == nil {
			return nil
		}

		if owner.APIVersion != mpasv1alpha1.GroupVersion.String() || owner.Kind != "Validation" {
			return nil
		}

		return []string{owner.Name}
	}); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&mpasv1alpha1.Validation{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&sourcebeta2.GitRepository{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=validations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=validations/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=validations/finalizers,verbs=update
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=repositories,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=repositories/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ValidationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	obj := &mpasv1alpha1.Validation{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to retrieve validation object: %w", err)
	}

	logger.V(4).Info("reconciling the validation", "status", obj.Status)

	objPatcher := patch.NewSerialPatcher(obj, r.Client)

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		// Patching has not been set up, or the controller errored earlier.
		if objPatcher == nil {
			return
		}

		// Set status observed generation option if the object is stalled or ready.
		if conditions.IsStalled(obj) || conditions.IsReady(obj) {
			obj.Status.ObservedGeneration = obj.Generation
		}

		if perr := objPatcher.Patch(ctx, obj); perr != nil {
			err = errors.Join(err, perr)
		}
	}()

	owners := obj.OwnerReferences
	if len(owners) != 1 {
		return ctrl.Result{}, fmt.Errorf("expected one owner, got: %d", len(owners))
	}

	sync := &gitv1alpha1.Sync{}
	if err := r.Get(ctx, types.NamespacedName{Name: obj.Spec.SyncRef.Name, Namespace: obj.Spec.SyncRef.Namespace}, sync); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to get owner object: %w", err)
	}

	if !conditions.IsReady(sync) || sync.Status.PullRequestID == 0 {
		logger.Info("sync request isn't done yet... waiting")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	projectList := &projectv1.ProjectList{}
	if err := r.List(ctx, projectList, client.InNamespace(r.MpasSystemNamespace)); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.ProjectInNamespaceGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find project in namespace: %w", err)
	}

	if v := len(projectList.Items); v != 1 {
		err := fmt.Errorf("exactly one Project should have been found in namespace %s; got: %d", obj.Namespace, v)
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.ProjectInNamespaceGetFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	//TODO: This is done in another place as well. Might have to extract it. Something like, getProjectForNamespace.
	project := &projectList.Items[0]

	if !conditions.IsReady(project) {
		logger.Info("project not ready yet")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	if project.Status.RepositoryRef == nil {
		logger.Info("no repository information is provided yet")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	repository := &gitmpasv1alpha1.Repository{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      project.Status.RepositoryRef.Name,
		Namespace: project.Status.RepositoryRef.Namespace,
	}, repository); err != nil {
		return ctrl.Result{}, err
	}

	if obj.Status.GitRepositoryRef == nil {
		logger.Info("creating git repository to track value changes")
		// create gitrepository to track values file and immediately requeue
		ref, err := r.createValueFileGitRepository(ctx, obj, owners[0].Name, sync.Status.PullRequestID, *repository)

		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create value tracking git repo: %w", err)
		}

		obj.Status.GitRepositoryRef = &ref

		return ctrl.Result{Requeue: true}, nil
	}

	// TODO: Do the validation
	for _, rule := range obj.Spec.ValidationRules {
		logger.Info("validating rule", "rule", string(rule.Data), "resource", rule.Name)
	}

	if err := r.deleteGitRepository(ctx, obj.Status.GitRepositoryRef); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.GitRepositoryCleanUpFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to delete GitRepository tracking the values file: %w", err)
	}

	if err := r.Validator.PassValidation(ctx, *repository, *sync); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.ValidationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to set pull request checks to success: %w", err)
	}

	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	return ctrl.Result{}, nil
}

// createValueFileGitRepository creates a GitRepository that tracks changes on a branch.
func (r *ValidationReconciler) createValueFileGitRepository(ctx context.Context, obj *mpasv1alpha1.Validation, productName string, pullId int, repository gitmpasv1alpha1.Repository) (meta.NamespacedObjectReference, error) {
	repo := &sourcebeta2.GitRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-values-repo",
			Namespace: obj.Namespace,
		},
		Spec: sourcebeta2.GitRepositorySpec{
			URL: repository.GetRepositoryURL(),
			SecretRef: &meta.LocalObjectReference{
				Name: repository.Spec.Credentials.SecretRef.Name,
			},
			Interval: metav1.Duration{Duration: 5 * time.Second},
			Reference: &sourcebeta2.GitRepositoryRef{
				Name: fmt.Sprintf("refs/pull/%d/head", pullId),
			},
			Ignore: pointer.String(fmt.Sprintf(`# exclude all
/*
# include values.yaml
!./products/%s/values.yaml
`, productName)),
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, repo, func() error {
		if repo.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetControllerReference(obj, repo, r.Scheme); err != nil {
				return fmt.Errorf("failed to set controller to git repository object: %w", err)
			}
		}

		return nil
	}); err != nil {
		return meta.NamespacedObjectReference{}, fmt.Errorf("failed to create git repository: %w", err)
	}

	return meta.NamespacedObjectReference{
		Name:      repo.Name,
		Namespace: repo.Namespace,
	}, nil
}

func (r *ValidationReconciler) deleteGitRepository(ctx context.Context, ref *meta.NamespacedObjectReference) error {
	repo := &sourcebeta2.GitRepository{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      ref.Name,
		Namespace: ref.Namespace,
	}, repo); err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}

		return fmt.Errorf("failed to find GitRepositroy object: %w", err)
	}

	return r.Delete(ctx, repo)
}
