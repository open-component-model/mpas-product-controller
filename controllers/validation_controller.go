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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	gitv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	gitmpasv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	"github.com/open-component-model/ocm-controller/pkg/status"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/internal/cue"
	"github.com/open-component-model/mpas-product-controller/pkg/validators"
)

const defaultValidationRequeue = 5 * time.Second

// ValidationReconciler reconciles a Validation object.
type ValidationReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	EventRecorder record.EventRecorder

	MpasSystemNamespace string
	Validator           validators.Validator
}

// SetupWithManager sets up the controller with the Manager.
func (r *ValidationReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&sourcebeta2.GitRepository{},
		controllerMetadataKey,
		func(rawObj client.Object) []string {
			repository, ok := rawObj.(*sourcebeta2.GitRepository)
			if !ok {
				return nil
			}

			owner := metav1.GetControllerOf(repository)
			if owner == nil {
				return nil
			}

			if owner.APIVersion != mpasv1alpha1.GroupVersion.String() || owner.Kind != "Validation" {
				return nil
			}

			return []string{owner.Name}
		},
	); err != nil {
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

	logger.V(mpasv1alpha1.LevelDebug).Info("reconciling the validation", "status", obj.Status)

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

	// These will be outputs of the below should run method.
	var (
		sync          = &gitv1alpha1.Sync{}
		repository    = &gitmpasv1alpha1.Repository{}
		gitRepository = &sourcebeta2.GitRepository{}
	)

	run, result, err := r.shouldRun(ctx, owners, obj, sync, repository, gitRepository)
	if err != nil {
		return ctrl.Result{}, err
	}
	if !run {
		return result, nil
	}

	artifact := gitRepository.GetArtifact()
	if artifact == nil {
		logger.Info("no artifact for values tracking git repository yet")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	if artifact.Digest == obj.Status.LastValidatedDigest {
		logger.Info("digest already validated", "digest", artifact.Digest)

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	obj.Status.LastValidatedDigest = artifact.Digest

	data, err := FetchValuesFileContent(ctx, owners[0].Name, artifact)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to fetch artifact content from git repository: %w", err)
	}

	config, err := cue.New("config", "", data)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, mpasv1alpha1.ValidationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to generate schema: %w", err)
	}

	schema, err := cue.New("schema", "", obj.Spec.Schema)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, mpasv1alpha1.ValidationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to generate schema: %w", err)
	}

	err = config.Validate(schema)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, mpasv1alpha1.ValidationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to validate config: %w", err)
	}

	if err := r.Validator.PassValidation(ctx, *repository, *sync); err != nil {
		status.MarkNotReady(r.EventRecorder, obj, mpasv1alpha1.ValidationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to set pull request checks to success: %w", err)
	}

	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")
	obj.Status.LastValidatedDigestOutcome = mpasv1alpha1.SuccessValidationOutcome

	// Requeue until the related pull request is merged or closed.
	return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
}

// createValueFileGitRepository creates a GitRepository that tracks changes on a branch.
func (r *ValidationReconciler) createValueFileGitRepository(
	ctx context.Context,
	obj *mpasv1alpha1.Validation,
	productName string,
	pullID int,
	repository gitmpasv1alpha1.Repository,
) (meta.NamespacedObjectReference, error) {
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
			Interval: metav1.Duration{Duration: defaultValidationRequeue},
			Reference: &sourcebeta2.GitRepositoryRef{
				Name: fmt.Sprintf("refs/pull/%d/head", pullID),
			},
			Ignore: pointer.String(fmt.Sprintf(`# exclude all
/*
# include values.yaml
!/products/%s/config.cue
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

		return fmt.Errorf("failed to find GitRepository object: %w", err)
	}

	return r.Delete(ctx, repo)
}

func (r *ValidationReconciler) shouldRun(
	ctx context.Context,
	owners []metav1.OwnerReference,
	obj *mpasv1alpha1.Validation,
	sync *gitv1alpha1.Sync,
	repository *gitmpasv1alpha1.Repository,
	gitRepository *sourcebeta2.GitRepository,
) (bool, ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if err := r.Get(ctx, types.NamespacedName{Name: obj.Spec.SyncRef.Name, Namespace: obj.Spec.SyncRef.Namespace}, sync); err != nil {
		if apierrors.IsNotFound(err) {
			return false, ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
		}

		return false, ctrl.Result{}, fmt.Errorf("failed to get owner object: %w", err)
	}

	if !conditions.IsReady(sync) || sync.Status.PullRequestID == 0 {
		logger.Info("sync request isn't done yet... waiting")

		return false, ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	project, err := GetProjectFromObjectNamespace(ctx, r.Client, sync, r.MpasSystemNamespace)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.ProjectInNamespaceGetFailedReason, err.Error())
		status.MarkNotReady(r.EventRecorder, obj, mpasv1alpha1.ProjectInNamespaceGetFailedReason, err.Error())

		return false, ctrl.Result{}, fmt.Errorf("failed to find the project in the namespace: %w", err)
	}

	if !conditions.IsReady(project) {
		logger.Info("project not ready yet")

		return false, ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	if project.Status.RepositoryRef == nil {
		logger.Info("no repository information is provided yet")

		return false, ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	if err := r.Get(ctx, types.NamespacedName{
		Name:      project.Status.RepositoryRef.Name,
		Namespace: project.Status.RepositoryRef.Namespace,
	}, repository); err != nil {
		return false, ctrl.Result{}, err
	}

	if conditions.IsTrue(obj, meta.ReadyCondition) {
		merged, err := r.Validator.IsMergedOrClosed(ctx, *repository, *sync)
		if err != nil {
			return false, ctrl.Result{}, fmt.Errorf("failed to fetch pull request status: %w", err)
		}

		if merged {
			logger.Info("validation pull request is merged/closed, removing git repository")
			if err := r.deleteGitRepository(ctx, obj.Status.GitRepositoryRef); err != nil {
				status.MarkNotReady(r.EventRecorder, obj, mpasv1alpha1.GitRepositoryCleanUpFailedReason, err.Error())

				return false, ctrl.Result{}, fmt.Errorf("failed to delete GitRepository tracking the values file: %w", err)
			}

			// Stop reconciling this validation any further.
			return false, ctrl.Result{}, nil
		}
	}

	if obj.Status.GitRepositoryRef == nil {
		logger.Info("creating git repository to track value changes")
		// create gitrepository to track values file and immediately requeue
		ref, err := r.createValueFileGitRepository(ctx, obj, owners[0].Name, sync.Status.PullRequestID, *repository)
		if err != nil {
			return false, ctrl.Result{}, fmt.Errorf("failed to create value tracking git repo: %w", err)
		}

		obj.Status.GitRepositoryRef = &ref

		// This will requeue anyway, since we are watching GitRepository objects. One the GitRepository is IsReady
		// it should requeue a run for this validation.
		return false, ctrl.Result{}, nil
	}

	if err := r.Get(ctx, types.NamespacedName{
		Name:      obj.Status.GitRepositoryRef.Name,
		Namespace: obj.Status.GitRepositoryRef.Namespace,
	}, gitRepository); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("values file tracking gitrepository already removed")

			return false, ctrl.Result{}, nil
		}

		return false, ctrl.Result{}, fmt.Errorf("failed to find values file tracking git repository: %w", err)
	}

	return true, ctrl.Result{}, nil
}
