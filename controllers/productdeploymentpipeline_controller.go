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
	"github.com/fluxcd/pkg/runtime/patch"
	rreconcile "github.com/fluxcd/pkg/runtime/reconcile"
	"github.com/open-component-model/ocm-controller/pkg/status"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	mpasocm "github.com/open-component-model/mpas-product-controller/pkg/ocm"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
)

const (
	defaultPipelineRequeue = 10 * time.Minute
	quickRequeue           = 10 * time.Second
)

// ProductDeploymentPipelineReconciler reconciles a ProductDeploymentPipeline object.
type ProductDeploymentPipelineReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	OCMClient           mpasocm.Contract
	MpasSystemNamespace string
	EventRecorder       record.EventRecorder
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

	patchHelper := patch.NewSerialPatcher(obj, r.Client)

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		// Patching has not been set up, or the controller errored earlier.
		if patchHelper == nil {
			return
		}

		if derr := status.UpdateStatus(ctx, patchHelper, obj, r.EventRecorder, 0); derr != nil {
			err = errors.Join(err, derr)
		}
	}()

	// Starts the progression by setting ReconcilingCondition.
	// This will be checked in defer.
	// Should only be deleted on a success.
	rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason, "reconciliation in progress for resource: %s", obj.Name)

	var snapshotProvider ocmv1alpha1.SnapshotWriter
	// Create Localization
	localization, err := r.createOrUpdateLocalization(ctx, obj)
	if err != nil {
		err := fmt.Errorf("failed to create localization: %w", err)
		status.MarkNotReady(r.EventRecorder, obj, mpasv1alpha1.CreateLocalizationFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	snapshotProvider = localization

	// Create Configuration
	configuration, err := r.createOrUpdateConfiguration(ctx, obj, localization)
	if err != nil {
		err := fmt.Errorf("failed to create configuration: %w", err)
		status.MarkNotReady(r.EventRecorder, obj, mpasv1alpha1.CreateConfigurationFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	if configuration != nil {
		snapshotProvider = configuration
	}

	if snapshotProvider == nil {
		err := fmt.Errorf("no artifact provider after localization and configuration")
		status.MarkNotReady(r.EventRecorder, obj, "NoArtifactAfterLocalizationAndConfiguration", err.Error())

		return ctrl.Result{}, err
	}

	if snapshotProvider.GetSnapshotName() == "" {
		logger.Info("snapshot hasn't been produced yet, requeuing pipeline", "pipeline", obj.Name)

		return ctrl.Result{RequeueAfter: quickRequeue}, nil
	}

	obj.Status.SnapshotRef = &meta.NamespacedObjectReference{
		Name:      snapshotProvider.GetSnapshotName(),
		Namespace: obj.Namespace,
	}

	status.MarkReady(r.EventRecorder, obj, "Reconciliation success")

	return ctrl.Result{}, nil
}

func (r *ProductDeploymentPipelineReconciler) createOrUpdateConfiguration(
	ctx context.Context,
	obj *mpasv1alpha1.ProductDeploymentPipeline,
	localization *ocmv1alpha1.Localization,
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

	spec := ocmv1alpha1.MutationSpec{
		Interval:  metav1.Duration{Duration: defaultPipelineRequeue},
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
			ConfigMapSource: &ocmv1alpha1.ConfigMapSource{
				SourceRef: meta.LocalObjectReference{
					Name: obj.Spec.ConfigMapRef,
				},
				Key: "values.yaml",
				//nolint:godox // yep
				// TODO: This means that's its a must have in the config.cue file
				// Reflect this in the cue file
				SubPath: obj.Name,
			},
		},
	}

	configuration := &ocmv1alpha1.Configuration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-configuration",
			Namespace: obj.Namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, configuration, func() error {
		if configuration.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, configuration, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to configuration object: %w", err)
			}
		}

		if configuration.Spec.SourceRef.ResourceRef == nil ||
			configuration.Spec.SourceRef.ResourceRef.Name != spec.SourceRef.ResourceRef.Name {
			configuration.Spec = spec
		}

		if configuration.Spec.ConfigRef.ResourceRef == nil ||
			configuration.Spec.ConfigRef.ResourceRef.Name != spec.ConfigRef.ResourceRef.Name {
			configuration.Spec = spec
		}

		if configuration.Spec.ValuesFrom.ConfigMapSource == nil ||
			configuration.Spec.ValuesFrom.ConfigMapSource.SourceRef.Name != spec.ValuesFrom.ConfigMapSource.SourceRef.Name {
			configuration.Spec = spec
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create configuration: %w", err)
	}

	return configuration, nil
}

func (r *ProductDeploymentPipelineReconciler) createOrUpdateLocalization(
	ctx context.Context,
	obj *mpasv1alpha1.ProductDeploymentPipeline,
) (*ocmv1alpha1.Localization, error) {
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
			Interval: metav1.Duration{Duration: defaultPipelineRequeue},
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
