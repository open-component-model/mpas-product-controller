// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"context"
	"crypto/sha1" //nolint:gosec // good enough for our purposes
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/jitter"
	"github.com/fluxcd/pkg/runtime/patch"
	rreconcile "github.com/fluxcd/pkg/runtime/reconcile"
	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/internal/cue"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/open-component-model/ocm-controller/pkg/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

const (
	controllerMetadataKey = ".metadata.controller"
)

// ProductDeploymentReconciler reconciles a ProductDeployment object.
type ProductDeploymentReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	EventRecorder       record.EventRecorder
	MpasSystemNamespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	err := r.indexProductDeploymentPipeline(mgr)
	if err != nil {
		return err
	}

	err = r.indexConfigMap(mgr)
	if err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProductDeployment{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Owns(&v1alpha1.ProductDeploymentPipeline{}).
		Complete(r)
}

func (*ProductDeploymentReconciler) indexConfigMap(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(context.Background(), &corev1.ConfigMap{}, controllerMetadataKey, func(rawObj client.Object) []string {
		cfg, ok := rawObj.(*corev1.ConfigMap)
		if !ok {
			return nil
		}

		labels := cfg.GetLabels()
		if labels == nil {
			return nil
		}

		owner, ok := labels[v1alpha1.ProductDeploymentOwnerLabelKey]
		if !ok {
			return nil
		}

		return []string{owner}
	})
}

func (*ProductDeploymentReconciler) indexProductDeploymentPipeline(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&v1alpha1.ProductDeploymentPipeline{},
		controllerMetadataKey,
		func(rawObj client.Object) []string {
			pipeline, ok := rawObj.(*v1alpha1.ProductDeploymentPipeline)
			if !ok {
				return nil
			}
			owner := metav1.GetControllerOf(pipeline)
			if owner == nil {
				return nil
			}

			if owner.APIVersion != v1alpha1.GroupVersion.String() || owner.Kind != v1alpha1.ProductDeploymentKind {
				return nil
			}

			return []string{owner.Name}
		},
	)
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeployments/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeployments/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Named return values: It's used for improving readability when dealing with the defer patch statement.
func (r *ProductDeploymentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, err error) {
	logger := log.FromContext(ctx)

	logger.V(v1alpha1.LevelDebug).Info("starting reconcile loop for product deployment")

	obj := &v1alpha1.ProductDeployment{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
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

	return r.reconcile(ctx, obj)
}

func (r *ProductDeploymentReconciler) reconcile(ctx context.Context, obj *v1alpha1.ProductDeployment) (result ctrl.Result, err error) {
	if obj.Generation != obj.Status.ObservedGeneration {
		rreconcile.ProgressiveStatus(
			false,
			obj,
			meta.ProgressingReason,
			"processing object: new generation %d -> %d",
			obj.Status.ObservedGeneration,
			obj.Generation,
		)
	}
	logger := log.FromContext(ctx)
	logger.Info("preparing to create pipeline objects")

	if err := r.createOrUpdateComponentVersion(ctx, obj); err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.CreateComponentVersionFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create component version: %w", err)
	}

	project, err := GetProjectFromObjectNamespace(ctx, r.Client, obj, r.MpasSystemNamespace)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ProjectInNamespaceGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find the project in the namespace: %w", err)
	}

	// If there is an error its either because we failed to list/create the configmap or we failed to delete the old ones.
	// In both cases we want to mark the object as not ready and requeue
	configName, configUpdated, err := r.createOrUpdatedValuesConfigMap(ctx, obj, project)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.CreateConfigMapFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to validate config: %w", err)
	}

	alreadyCreatedPipelines := make(map[string]struct{})

	existingPipelines := &v1alpha1.ProductDeploymentPipelineList{}
	if err := r.List(ctx, existingPipelines, client.InNamespace(obj.Namespace), client.MatchingFields{controllerMetadataKey: obj.Name}); err != nil {
		status.MarkNotReady(r.EventRecorder, obj, "ListingOwnedPipelinesFailed", err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to list owned pipelines: %w", err)
	}

	logger.Info("retrieved number of existing pipeline items", "items", len(existingPipelines.Items))

	obj.Status.ActivePipelines = nil
	for _, ep := range existingPipelines.Items {
		ep := ep
		alreadyCreatedPipelines[ep.Name] = struct{}{}

		if !conditions.IsTrue(&ep, meta.ReadyCondition) {
			obj.Status.ActivePipelines = append(obj.Status.ActivePipelines, ep.Name)
		}
	}

	// Done handling existing items, now create the rest.
	for _, pipeline := range obj.Spec.Pipelines {
		if _, ok := alreadyCreatedPipelines[pipeline.Name]; ok && !configUpdated {
			// already created, skip only if config was not updated due to the values not changing
			continue
		}

		spec := v1alpha1.ProductDeploymentPipelineSpec{
			Resource:            pipeline.Resource,
			Localization:        pipeline.Localization,
			Configuration:       pipeline.Configuration,
			TargetRole:          pipeline.TargetRole,
			ComponentVersionRef: r.generateComponentVersionName(obj),
			ConfigMapRef:        configName,
		}

		pobj := &v1alpha1.ProductDeploymentPipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pipeline.Name,
				Namespace: obj.Namespace,
				Labels: map[string]string{
					v1alpha1.ProductDeploymentOwnerLabelKey: obj.Name,
				},
			},
		}

		if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, pobj, func() error {
			if pobj.ObjectMeta.CreationTimestamp.IsZero() {
				if err := controllerutil.SetControllerReference(obj, pobj, r.Scheme); err != nil {
					return fmt.Errorf("failed to set owner to pipeline object object: %w", err)
				}
			}

			if !pobj.Equals(spec) {
				pobj.Spec = spec
			}

			return nil
		}); err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to create pipeline object: %w", err)
		}

		obj.Status.ActivePipelines = append(obj.Status.ActivePipelines, pobj.Name)

		logger.V(v1alpha1.LevelDebug).Info("pipeline object successfully created", "name", pobj.Name)
	}

	logger.Info("all pipelines handled successfully")
	status.MarkReady(r.EventRecorder, obj, "Reconciliation success")

	//nolint:godox // yep
	// TODO: do something with failed and successful pipelines
	return ctrl.Result{RequeueAfter: jitter.JitteredIntervalDuration(obj.GetRequeueAfter())}, nil
}

// createOrUpdatedValuesConfigMap generates the values.yaml file for the product deployment and returns the name of the configmap that
// contains the values.yaml file.
// It retrieves the existing config.cue, performs validation and generate a yaml representation of the values that is used
// to create the configmap.
func (r *ProductDeploymentReconciler) createOrUpdatedValuesConfigMap(
	ctx context.Context,
	obj *v1alpha1.ProductDeployment,
	project *projectv1.Project,
) (string, bool, error) {
	config, err := fetchExistingConfigFile(ctx, r.Client, obj.Name, project)
	if err != nil {
		return "", false, fmt.Errorf("failed to retrieve config: %w", err)
	}

	if config == nil {
		return "", false, fmt.Errorf("no config found")
	}

	schema, err := cue.New("schema", "", obj.Spec.Schema)
	if err != nil {
		return "", false, fmt.Errorf("failed to retrieve schema: %w", err)
	}

	err = config.Validate(schema)
	if err != nil {
		return "", false, fmt.Errorf("failed to validate config: %w", err)
	}

	values, err := config.Merge(schema)
	if err != nil {
		return "", false, fmt.Errorf("failed to merge config with schema: %w", err)
	}

	valuesYaml, err := values.Yaml()
	if err != nil {
		return "", false, fmt.Errorf("failed to convert config values to yaml: %w", err)
	}

	configName, configUpdated, err := r.generateConfigMap(ctx, obj, string(valuesYaml))
	if err != nil {
		return "", false, fmt.Errorf("failed to generate configmap: %w", err)
	}

	return configName, configUpdated, nil
}

func (r *ProductDeploymentReconciler) createOrUpdateComponentVersion(ctx context.Context, obj *v1alpha1.ProductDeployment) error {
	signature := make([]ocmv1alpha1.Signature, 0, len(obj.Spec.Verify))
	for _, s := range obj.Spec.Verify {
		sign := ocmv1alpha1.Signature{
			Name: s.Name,
			PublicKey: ocmv1alpha1.PublicKey{
				SecretRef: s.PublicKey.SecretRef,
				Value:     s.PublicKey.Value,
			},
		}
		signature = append(signature, sign)
	}

	cv := &ocmv1alpha1.ComponentVersion{
		ObjectMeta: metav1.ObjectMeta{
			Name:      r.generateComponentVersionName(obj),
			Namespace: obj.Namespace,
		},
		Spec: ocmv1alpha1.ComponentVersionSpec{
			// Note: The interval here doesn't matter because we always pin to a specific version anyway.
			Interval:  metav1.Duration{Duration: defaultPipelineRequeue},
			Component: obj.Spec.Component.Name,
			Repository: ocmv1alpha1.Repository{
				URL: obj.Spec.Component.Registry.URL,
			},
			Verify: signature,
			References: ocmv1alpha1.ReferencesConfig{
				Expand: true,
			},
			ServiceAccountName: obj.Spec.ServiceAccountName,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, cv, func() error {
		if cv.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, cv, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to sync object: %w", err)
			}
		}

		cv.Spec.Version = ocmv1alpha1.Version{
			Semver: obj.Spec.Component.Version,
		}

		// We need to update the replication-controller verification public key.
		cv.Spec.Verify = signature

		return nil
	}); err != nil {
		return fmt.Errorf("failed to create component version: %w", err)
	}

	return nil
}

func (r *ProductDeploymentReconciler) generateComponentVersionName(obj *v1alpha1.ProductDeployment) string {
	return obj.Name + "component-version"
}

func (r *ProductDeploymentReconciler) generateConfigMap(ctx context.Context, obj *v1alpha1.ProductDeployment, values string) (string, bool, error) {
	existingMaps := &corev1.ConfigMapList{}
	if err := r.List(ctx, existingMaps, client.InNamespace(obj.Namespace), client.MatchingFields{controllerMetadataKey: obj.Name}); err != nil {
		return "", false, fmt.Errorf("failed to list owned configmaps: %w", err)
	}

	configName := obj.Name + "-values-" + hashValues(values)
	for _, cm := range existingMaps.Items {
		if cm.Name == configName {
			return configName, false, nil
		}
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configName,
			Namespace: obj.Namespace,
			Labels: map[string]string{
				v1alpha1.ProductDeploymentOwnerLabelKey: obj.Name,
			},
		},
		Data: map[string]string{
			"values.yaml": values,
		},
	}

	if err := r.Create(ctx, configMap); err != nil {
		return configName, false, fmt.Errorf("failed to create configmap: %w", err)
	}

	if err := controllerutil.SetOwnerReference(obj, configMap, r.Scheme); err != nil {
		return configName, true, fmt.Errorf("failed to set owner to configmap object: %w", err)
	}

	// garbage collect old configmaps
	var errf error
	for _, cm := range existingMaps.Items {
		cm := cm
		err := r.Client.Delete(ctx, &cm)
		if err != nil {
			errf = errors.Join(errf, err)
		}
	}

	return configName, true, errf
}

func hashValues(values string) string {
	h := sha1.New() //nolint:gosec // good enough for our purposes
	h.Write([]byte(values))

	return hex.EncodeToString(h.Sum(nil))[:8]
}
