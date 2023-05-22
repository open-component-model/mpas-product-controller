// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	kustomizev1beta2 "github.com/fluxcd/kustomize-controller/api/v1beta2"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	v1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/fluxcd/source-controller/api/v1beta2"
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

	project := &projectList.Items[0]

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

	// Create Flux OCI
	if err := r.createOrUpdateFluxOCIRepository(ctx, obj, snapshotProvider); err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("waiting for artifact to be available from either configuration or localization for pipeline", "pipeline", obj.Name)

			return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
		}
		err := fmt.Errorf("failed to create oci repository: %w", err)
		conditions.MarkFalse(obj, meta.ReadyCondition, mpasv1alpha1.CreateOCIRepositoryFailedReason, err.Error())

		return ctrl.Result{}, err
	}

	// if target Kubernetes -> Create a Kustomize and point it at the OCI Repository.
	if obj.Spec.TargetRole.Type == mpasv1alpha1.Kubernetes {
		target, err := FilterTarget(ctx, r.Client, obj.Spec.TargetRole, obj.Namespace)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to select a target with the given criteria: %w", err)
		}

		// Update the selected target.
		obj.Status.SelectedTarget = target

		if err := r.createOrUpdateKustomization(ctx, obj, target); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to generate target kustomization: %w", err)
		}
	}

	conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")

	return ctrl.Result{}, nil
}

func (r *ProductDeploymentPipelineReconciler) createOrUpdateConfiguration(ctx context.Context, obj *mpasv1alpha1.ProductDeploymentPipeline, owner *mpasv1alpha1.ProductDeployment, localization *ocmv1alpha1.Localization, project *projectv1.Project) (*ocmv1alpha1.Configuration, error) {
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

	// Entry ID: <namespace>_<name>_<group>_<kind>. Just look for a postfix of gitrepository
	if project.Status.Inventory == nil {
		return nil, fmt.Errorf("project inventory is empty")
	}

	var repoName, repoNamespace string
	for _, e := range project.Status.Inventory.Entries {
		split := strings.Split(e.ID, "_")
		if len(split) < 1 {
			return nil, fmt.Errorf("failed to split ID: %s", e.ID)
		}

		if split[len(split)-1] == v1.GitRepositoryKind {
			repoName = split[1]
			repoNamespace = split[0]
			break
		}
	}

	if repoName == "" {
		return nil, fmt.Errorf("gitrepository not found in the project inventory")
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

func (r *ProductDeploymentPipelineReconciler) createOrUpdateFluxOCIRepository(ctx context.Context, obj *mpasv1alpha1.ProductDeploymentPipeline, snapshotProvider ocmv1alpha1.SnapshotWriter) error {
	snapshot := &ocmv1alpha1.Snapshot{}
	if err := r.Get(ctx, types.NamespacedName{Name: snapshotProvider.GetSnapshotName(), Namespace: obj.Namespace}, snapshot); err != nil {
		return fmt.Errorf("failed to find snapshot: %w", err)
	}

	url := strings.ReplaceAll(snapshot.Status.RepositoryURL, "http", "oci")
	repo := &v1beta2.OCIRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-oci-repository",
			Namespace: obj.Namespace,
		},
		Spec: v1beta2.OCIRepositorySpec{
			URL: url,
			Reference: &v1beta2.OCIRepositoryRef{
				Tag: snapshot.Spec.Tag,
			},
			Interval: metav1.Duration{Duration: 10 * time.Minute},
			Insecure: true,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, repo, func() error {
		if repo.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, repo, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to oci repository object: %w", err)
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to create oci repository: %w", err)
	}

	return nil
}

func (r *ProductDeploymentPipelineReconciler) createOrUpdateKustomization(ctx context.Context, obj *mpasv1alpha1.ProductDeploymentPipeline, target *mpasv1alpha1.Target) error {
	if target.Spec.Access == nil {
		return fmt.Errorf("access needs to be defined for the kubernetes target type")
	}

	kustomization := &kustomizev1beta2.Kustomization{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-kustomization",
			Namespace: obj.Namespace,
		},
		Spec: kustomizev1beta2.KustomizationSpec{
			KubeConfig: &meta.KubeConfigReference{
				SecretRef: meta.SecretKeyReference{
					Name: target.Spec.Access.SecretRef.Name,
				},
			},
			Prune: true,
			SourceRef: kustomizev1beta2.CrossNamespaceSourceReference{
				Kind:      "OCIRepository",
				Name:      obj.Name + "-oci-repository", // TODO: Maybe get this from the generated oci repo.
				Namespace: obj.Namespace,                // TODO: This is the same as the owner.
			},
			TargetNamespace: obj.Namespace, //TODO: This needs to come from somewhere.
			Timeout:         nil,           // TODO: Probably need to set this together with retry.
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, kustomization, func() error {
		if kustomization.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, kustomization, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to kustomization object: %w", err)
			}
		}

		return nil
	}); err != nil {
		return fmt.Errorf("failed to create kustomization: %w", err)
	}

	return nil
}
