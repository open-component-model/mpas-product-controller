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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
)

// ProductDeploymentPipelineReconciler reconciles a ProductDeploymentPipeline object.
type ProductDeploymentPipelineReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	MpasSystemNamespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentPipelineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mpasv1alpha1.ProductDeploymentPipeline{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentpipelines,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentpipelines/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentpipelines/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProductDeploymentPipelineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	logger.Info("reconciling pipeline object", "pipeline", req.NamespacedName)

	obj := &mpasv1alpha1.ProductDeploymentPipeline{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to find pipeline deployment object: %w", err)
	}

	owners := obj.GetOwnerReferences()
	if len(owners) != 1 {
		return ctrl.Result{}, fmt.Errorf("expected exactly one owner, got: %d", len(owners))
	}

	owner := &mpasv1alpha1.ProductDeployment{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      owners[0].Name,
		Namespace: obj.Namespace,
	}, owner); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to find the owner: %w", err)
	}

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

	var snapshotProvider ocmv1alpha1.SnapshotWriter
	// Create Localization
	localization, err := r.createOrUpdateLocalization(ctx, obj)
	if err != nil {
		err := fmt.Errorf("failed to create localization: %w", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.CreateLocalizationFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	snapshotProvider = localization

	project, err := GetProjectInNamespace(ctx, r.Client, r.MpasSystemNamespace)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.ProjectInNamespaceGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find the project in the namespace: %w", err)
	}

	// Create Configuration
	configuration, err := r.createOrUpdateConfiguration(ctx, obj, owner, localization, project)
	if err != nil {
		err := fmt.Errorf("failed to create configuration: %w", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.CreateConfigurationFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	if configuration != nil {
		snapshotProvider = configuration
	}

	if snapshotProvider == nil {
		return ctrl.Result{}, fmt.Errorf("no artifact provider after localization and configuration")
	}

	if snapshotProvider.GetSnapshotName() == "" {
		logger.Info("snapshot hasn't been produced yet, requeuing pipeline", "pipeline", obj.Name)

		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	obj.Status.SnapshotRef = &meta.NamespacedObjectReference{
		Name:      snapshotProvider.GetSnapshotName(),
		Namespace: obj.Namespace,
	}

	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	return ctrl.Result{}, nil
}

func (r *ProductDeploymentPipelineReconciler) createOrUpdateConfiguration(
	ctx context.Context,
	obj *mpasv1alpha1.ProductDeploymentPipeline,
	owner *mpasv1alpha1.ProductDeployment,
	localization *ocmv1alpha1.Localization,
	project *projectv1.Project,
) (*ocmv1alpha1.Configuration, error) {
	if obj.Spec.Configuration.Rules.Name == "" {
		return nil, nil
	}

	reference := ocmv1alpha1.ResourceReference(obj.Spec.Configuration.Rules)
	sourceResource := ocmv1alpha1.ResourceReference(obj.Spec.Resource)
	source := ocmv1alpha1.ObjectReference{
		NamespacedObjectKindReference: meta.NamespacedObjectKindReference{
			Kind:      "ComponentVersion",
			Name:      obj.Spec.ComponentVersionRef,
			Namespace: obj.Namespace,
		},
		ResourceRef: &sourceResource,
	}

	if localization != nil {
		source = ocmv1alpha1.ObjectReference{
			NamespacedObjectKindReference: meta.NamespacedObjectKindReference{
				Kind:      "Localization",
				Name:      localization.Name,
				Namespace: obj.Namespace,
			},
		}
	}

	// get the git repository of the created repository
	repoName, repoNamespace, err := FetchGitRepositoryFromProjectInventory(project)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch GitRepository from project: %w", err)
	}

	configuration := &ocmv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-configuration",
			Namespace: obj.Namespace,
		},
		Spec: ocmv1alpha1.MutationSpec{
			Interval:  metav1.Duration{Duration: 10 * time.Minute},
			SourceRef: source,
			ConfigRef: &ocmv1alpha1.ObjectReference{
				NamespacedObjectKindReference: meta.NamespacedObjectKindReference{
					Kind:      "ComponentVersion",
					Name:      obj.Spec.ComponentVersionRef,
					Namespace: obj.Namespace,
				},
				ResourceRef: &reference,
			},
			ValuesFrom: &ocmv1alpha1.ValuesSource{
				FluxSource: &ocmv1alpha1.FluxValuesSource{
					SourceRef: meta.NamespacedObjectKindReference{
						Kind:      "GitRepository",
						Name:      repoName,
						Namespace: repoNamespace,
					},
					Path:    "./products/" + owner.Name + "/values.yaml",
					SubPath: obj.Name,
				},
			},
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, configuration, func() error {
		if configuration.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, configuration, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to configuration object: %w", err)
			}
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create configuration: %w", err)
	}

	return configuration, nil
}

func (r *ProductDeploymentPipelineReconciler) createOrUpdateLocalization(ctx context.Context, obj *mpasv1alpha1.ProductDeploymentPipeline) (*ocmv1alpha1.Localization, error) {
	if obj.Spec.Localization.Name == "" {
		return nil, nil
	}

	reference := ocmv1alpha1.ResourceReference(obj.Spec.Localization)
	source := ocmv1alpha1.ResourceReference(obj.Spec.Resource)

	localization := &ocmv1alpha1.Localization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-localization",
			Namespace: obj.Namespace,
		},
		Spec: ocmv1alpha1.MutationSpec{
			Interval: metav1.Duration{Duration: 10 * time.Minute},
			SourceRef: ocmv1alpha1.ObjectReference{
				NamespacedObjectKindReference: meta.NamespacedObjectKindReference{
					Kind:      "ComponentVersion",
					Name:      obj.Spec.ComponentVersionRef,
					Namespace: obj.Namespace,
				},
				ResourceRef: &source,
			},
			ConfigRef: &ocmv1alpha1.ObjectReference{
				NamespacedObjectKindReference: meta.NamespacedObjectKindReference{
					Kind:      "ComponentVersion",
					Name:      obj.Spec.ComponentVersionRef,
					Namespace: obj.Namespace,
				},
				ResourceRef: &reference,
			},
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, localization, func() error {
		if localization.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, localization, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to localization object: %w", err)
			}
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create localization: %w", err)
	}

	return localization, nil
}
