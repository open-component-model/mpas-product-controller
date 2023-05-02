// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	v1alpha12 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/open-component-model/ocm/pkg/common/compression"
	"github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/versions/ocm.software/v3alpha1"

	gitv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	"github.com/open-component-model/ocm-controller/pkg/snapshot"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/pkg/ocm"
)

const (
	// ProductDescriptionType defines the type of the ProductDescription resource in the component version.
	ProductDescriptionType = "productdescription.mpas.ocm.software"

	// MaximumNumberOfConcurrentPipelineRuns defines how many concurrent running pipelines there can be.
	MaximumNumberOfConcurrentPipelineRuns = 10
)

// ProductDeploymentGeneratorReconciler reconciles a ProductDeploymentGenerator object
type ProductDeploymentGeneratorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	OCMClient      ocm.Contract
	SnapshotWriter snapshot.Writer
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators/finalizers,verbs=update
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentversions;componentdescriptors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=snapshots,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=snapshots/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=snapshots/finalizers,verbs=update
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=syncs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *ProductDeploymentGeneratorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	logger.V(4).Info("starting product deployment generator loop")

	obj := &v1alpha1.ProductDeploymentGenerator{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("failed to get product deployment generator: %w", err)
	}

	var patchHelper *patch.Helper
	if patchHelper, err = patch.NewHelper(obj, r.Client); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create patch helper: %w", err)
	}

	// Always attempt to patch the object and status after each reconciliation.
	defer func() {
		// Patching has not been set up, or the controller errored earlier.
		if patchHelper == nil {
			return
		}

		if condition := conditions.Get(obj, meta.StalledCondition); condition != nil && condition.Status == metav1.ConditionTrue {
			conditions.Delete(obj, meta.ReconcilingCondition)
		}

		// Check if it's a successful reconciliation.
		// We don't set Requeue in case of error, so we can safely check for Requeue.
		if result.RequeueAfter == obj.GetRequeueAfter() && !result.Requeue && err == nil {
			// Remove the reconciling condition if it's set.
			conditions.Delete(obj, meta.ReconcilingCondition)

			// Set the return err as the ready failure message if the resource is not ready, but also not reconciling or stalled.
			if ready := conditions.Get(obj, meta.ReadyCondition); ready != nil && ready.Status == metav1.ConditionFalse && !conditions.IsStalled(obj) {
				err = errors.New(conditions.GetMessage(obj, meta.ReadyCondition))
			}
		}

		// If still reconciling then reconciliation did not succeed, set to ProgressingWithRetry to
		// indicate that reconciliation will be retried.
		if conditions.IsReconciling(obj) {
			reconciling := conditions.Get(obj, meta.ReconcilingCondition)
			reconciling.Reason = meta.ProgressingWithRetryReason
			conditions.Set(obj, reconciling)
		}

		// If not reconciling or stalled than mark Ready=True
		if !conditions.IsReconciling(obj) &&
			!conditions.IsStalled(obj) &&
			err == nil &&
			result.RequeueAfter == obj.GetRequeueAfter() {
			conditions.MarkTrue(obj, meta.ReadyCondition, meta.SucceededReason, "Reconciliation success")
		}
		// Set status observed generation option if the component is stalled or ready.
		if conditions.IsStalled(obj) || conditions.IsReady(obj) {
			obj.Status.ObservedGeneration = obj.Generation
		}

		// Update the object.
		if perr := patchHelper.Patch(ctx, obj); perr != nil {
			err = errors.Join(err, perr)
		}
	}()

	// Remove any stale Ready condition, most likely False, set above. Its value
	// is derived from the overall result of the reconciliation in the deferred
	// block at the very end.
	conditions.Delete(obj, meta.ReadyCondition)

	return r.reconcile(ctx, obj)
}

func (r *ProductDeploymentGeneratorReconciler) reconcile(ctx context.Context, obj *v1alpha1.ProductDeploymentGenerator) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	subscription := &replicationv1.ComponentSubscription{}

	if err := r.Get(ctx, types.NamespacedName{
		Name:      obj.Spec.SubscriptionRef.Name,
		Namespace: obj.Spec.SubscriptionRef.Namespace,
	}, subscription); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ComponentSubscriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find subscription object: %w", err)
	}

	if !conditions.IsReady(subscription) {
		logger.Info("referenced subscription isn't ready yet, requeuing")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	// TODO: Get the project and get Repository information from there.

	component := subscription.GetComponentVersion()
	logger.Info("fetching component", "component", component)

	octx, err := r.OCMClient.CreateAuthenticatedOCMContext(ctx, obj.Spec.ServiceAccountName, obj.Namespace)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.OCMAuthenticationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to authenticate using service account: %w", err)
	}

	cv, err := r.OCMClient.GetComponentVersion(ctx, octx, component.Registry.URL, component.Name, component.Version)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ComponentVersionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to authenticate using service account: %w", err)
	}

	logger.Info("retrieved component version, fetching ProductDescription resource", "component", cv.GetName())

	resources, err := cv.GetDescriptor().GetResourcesByType(ProductDescriptionType)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find the product description on the component: %w", err)
	}

	if resources.Len() != 1 {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.NumberOfProductDescriptionsInComponentIncorrectReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("number of product descriptions in component should be 1 but was: %d", resources.Len())
	}

	resource := resources.Get(0)

	descriptor, err := cv.GetResource(resource.GetMeta().GetIdentity(resources))
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find the product description on the component: %w", err)
	}

	am, err := descriptor.AccessMethod()
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to get access method for product description: %w", err)
	}

	reader, err := am.Reader()
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to get product description content: %w", err)
	}

	decompressed, _, err := compression.AutoDecompress(reader)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to get decompressed reader: %w", err)
	}

	content, err := io.ReadAll(decompressed)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to read content for product description: %w", err)
	}

	prodDesc := &v1alpha1.ProductDescription{}
	if err := yaml.Unmarshal(content, prodDesc); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to unmarshal product description: %w", err)
	}

	logger.Info("fetched product description", "description", klog.KObj(prodDesc))

	productDeployment := &v1alpha1.ProductDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      prodDesc.Name,
			Namespace: obj.Namespace,
		},
	}

	spec := v1alpha1.ProductDeploymentSpec{}
	spec.Component = component

	for _, p := range prodDesc.Spec.Pipelines {
		pipe, err := r.createProductPipeline(*prodDesc, p)
		if err != nil {
			conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

			return ctrl.Result{}, fmt.Errorf("failed to create product pipeline: %w", err)
		}
		spec.Pipelines = append(spec.Pipelines, pipe)
	}

	productDeployment.Spec = spec

	logger.Info("successfully generated product deployment", "productDeployment", klog.KObj(productDeployment))

	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, r.Scheme, r.Scheme, json.SerializerOptions{
		Yaml:   true,
		Pretty: true,
	})

	dir, err := os.MkdirTemp("", "product-deployment")
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create temp folder: %w", err)
	}

	defer os.RemoveAll(dir)

	productDeploymentFile, err := os.Create(filepath.Join(dir, "product-deployment.yaml"))
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create file: %w", err)
	}

	defer productDeploymentFile.Close()

	if err := serializer.Encode(productDeployment, productDeploymentFile); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to encode product deployment: %w", err)
	}

	snapshotName := obj.Name + "-snapshot"
	identity := v1alpha12.Identity{
		"ProductDeploymentName": obj.Name,
	}

	if _, err := r.SnapshotWriter.Write(ctx, v1alpha12.SnapshotTemplateSpec{
		Name: snapshotName,
	}, obj, dir, identity); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to write to the cache: %w", err)
	}

	// TODO: Figure out how to get the information on what repository object the git repository is based on.
	// TODO: Figure out commit message details. Maybe add a tempate?

	sync := &gitv1alpha1.Sync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-sync",
			Namespace: obj.Namespace,
		},
		Spec: gitv1alpha1.SyncSpec{
			SnapshotRef: corev1.LocalObjectReference{
				Name: snapshotName,
			},
			RepositoryRef: corev1.LocalObjectReference{ // TODO: for now create this by hand
				Name: obj.Spec.RepositoryRef.Name,
			},
			Interval: metav1.Duration{
				Duration: 1 * time.Minute,
			},
			CommitTemplate: gitv1alpha1.CommitTemplate{
				Name:         "<name>",
				Email:        "<email>",
				Message:      "New Product Deployment",
				TargetBranch: "main",
				BaseBranch:   "main",
			},
			AutomaticPullRequestCreation: true,
			SubPath:                      "generators/.",
		},
	}

	if err := r.Create(ctx, sync); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create sync request: %w", err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentGeneratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProductDeploymentGenerator{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Complete(r)
}

// createProductPipeline takes a pipeline description and builds up all the Kubernetes objects that are needed
// for that resource.
func (r *ProductDeploymentGeneratorReconciler) createProductPipeline(description v1alpha1.ProductDescription, p v1alpha1.ProductDescriptionPipeline) (v1alpha1.Pipeline, error) {
	// TODO: Select a target role from the based on the target selector.
	var targetRole *v1alpha1.TargetRole
	for _, role := range description.Spec.TargetRoles {
		if role.Name == p.TargetRoleName {
			targetRole = &role.TargetRole
			break
		}
	}
	if targetRole == nil {
		return v1alpha1.Pipeline{}, fmt.Errorf("failed to find a target role with name %s", p.TargetRoleName)
	}
	return v1alpha1.Pipeline{
		Name: p.Name,
		Localization: v1alpha1.Localization{
			Rules: v3alpha1.ElementMeta{
				Name:    p.Localization.Name,
				Version: p.Localization.Version,
			},
		},
		Configuration: v1alpha1.Configuration{
			Rules: v3alpha1.ElementMeta{
				Name:    p.Configuration.Name,
				Version: p.Configuration.Version,
			},
			ValuesFile: v1alpha1.ValuesFile{},
		},
		Resource: v3alpha1.ElementMeta{
			Name:    p.Source.Name,
			Version: p.Source.Version,
		},
		TargetRole: *targetRole,
	}, nil
}
