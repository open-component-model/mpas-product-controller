// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package controllers

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	rreconcile "github.com/fluxcd/pkg/runtime/reconcile"
	"github.com/fluxcd/source-controller/api/v1beta2"
	"github.com/open-component-model/mpas-product-controller/internal/cue"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	"github.com/open-component-model/ocm-controller/pkg/status"
	markdown "github.com/teekennedy/goldmark-markdown"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gitv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/open-component-model/ocm-controller/pkg/snapshot"
	"github.com/open-component-model/ocm/pkg/contexts/ocm"
	ocmmetav1 "github.com/open-component-model/ocm/pkg/contexts/ocm/compdesc/meta/v1"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	mpasocm "github.com/open-component-model/mpas-product-controller/pkg/ocm"
)

const (
	kustomizationFile = `resources:
- product-deployment.yaml
`
	componentVersionAnnotationKey = "mpas.ocm.system/component-version"
	componentNameAnnotationKey    = "mpas.ocm.system/component-name"
	ignoreMarker                  = "+mpas-ignore"
)

const (
	sourceKey = ".metadata.source"
)

var (
	unschedulableError = errors.New("pipeline cannot be scheduled as it doesn't define any target scopes")
)

// ProductDeploymentGeneratorReconciler reconciles a ProductDeploymentGenerator object
type ProductDeploymentGeneratorReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	OCMClient           mpasocm.Contract
	SnapshotWriter      snapshot.Writer
	MpasSystemNamespace string
	EventRecorder       record.EventRecorder
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentGeneratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &v1alpha1.ProductDeploymentGenerator{}, sourceKey, func(rawObj client.Object) []string {
		generator := rawObj.(*v1alpha1.ProductDeploymentGenerator)
		var ns = generator.Spec.SubscriptionRef.Namespace
		if ns == "" {
			ns = generator.GetNamespace()
		}
		return []string{fmt.Sprintf("%s/%s", ns, generator.Spec.SubscriptionRef.Name)}
	}); err != nil {
		return fmt.Errorf("failed setting index fields: %w", err)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.ProductDeploymentGenerator{}, builder.WithPredicates(predicate.GenerationChangedPredicate{})).
		Watches(
			&replicationv1.ComponentSubscription{},
			handler.EnqueueRequestsFromMapFunc(r.findGenerators),
			builder.WithPredicates(ComponentSubscriptionVersionChangedPredicate{})).
		Complete(r)
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators/finalizers,verbs=update
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=projects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=targets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets;serviceaccounts;namespaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentversions;componentdescriptors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=localizations;configurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=fluxdeployers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories;ocirepositories;buckets;ocirepositories,verbs=create;update;patch;delete;get;list;watch
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=snapshots,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=snapshots/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=snapshots/finalizers,verbs=update
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=syncs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete

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

func (r *ProductDeploymentGeneratorReconciler) reconcile(ctx context.Context, obj *v1alpha1.ProductDeploymentGenerator) (_ ctrl.Result, err error) {
	subscription := &replicationv1.ComponentSubscription{}

	if err := r.Get(ctx, types.NamespacedName{
		Name:      obj.Spec.SubscriptionRef.Name,
		Namespace: obj.Spec.SubscriptionRef.Namespace,
	}, subscription); err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ComponentSubscriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find subscription object: %w", err)
	}

	if !conditions.IsReady(subscription) {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ComponentSubscriptionNotReadyReason, "component subscription not ready yet")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	if obj.Status.LastReconciledVersion != "" {
		lastReconciledGeneratorVersion, err := semver.NewVersion(obj.Status.LastReconciledVersion)
		if err != nil {
			status.MarkNotReady(r.EventRecorder, obj, v1alpha1.SemverParseFailedReason, err.Error())

			return ctrl.Result{}, fmt.Errorf("failed to parse last reconciled version: %w", err)
		}

		lastReconciledSubscription, err := semver.NewVersion(subscription.Status.LastAppliedVersion)
		if err != nil {
			status.MarkNotReady(r.EventRecorder, obj, v1alpha1.SemverParseFailedReason, err.Error())

			return ctrl.Result{}, fmt.Errorf("failed to parse last reconciled version: %w", err)
		}

		if lastReconciledSubscription.Equal(lastReconciledGeneratorVersion) || lastReconciledSubscription.LessThan(lastReconciledGeneratorVersion) {
			status.MarkReady(r.EventRecorder, obj, "Reconciliation success")

			return ctrl.Result{}, nil
		}
	}

	project, err := GetProjectFromObjectNamespace(ctx, r.Client, obj, r.MpasSystemNamespace)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ProjectInNamespaceGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find the project in the namespace: %w", err)
	}

	if !conditions.IsReady(project) {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ProjectNotReadyReason, "project not ready yet")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	if project.Status.RepositoryRef == nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.RepositoryInformationMissingReason, "repository information missing")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	component := subscription.GetComponentVersion()
	rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason, "fetching component %s", component.Name)

	octx, err := r.OCMClient.CreateAuthenticatedOCMContext(ctx, obj.Spec.ServiceAccountName, obj.Namespace)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.OCMAuthenticationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to authenticate using service account: %w", err)
	}

	cv, err := r.OCMClient.GetComponentVersion(ctx, octx, component.Registry.URL, component.Name, component.Version)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ComponentVersionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to authenticate using service account: %w", err)
	}
	defer func() {
		if cerr := cv.Close(); cerr != nil {
			err = errors.Join(err, cerr)
		}
	}()

	rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason, "fetching component description %s", component.Name)

	content, err := r.OCMClient.GetProductDescription(ctx, octx, cv)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to get product description data: %w", err)
	}

	prodDesc := &v1alpha1.ProductDescription{}
	if err := kyaml.Unmarshal(content, prodDesc); err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to unmarshal product description: %w", err)
	}

	dir, err := os.MkdirTemp("", "product-deployment")
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.TemporaryFolderGenerationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create temp folder: %w", err)
	}

	defer func() {
		if oerr := os.RemoveAll(dir); oerr != nil {
			err = errors.Join(err, oerr)
		}
	}()

	// Create top-level product folder.
	if err := os.Mkdir(filepath.Join(dir, obj.Name), 0o777); err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.TemporaryFolderGenerationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create product folder: %w", err)
	}

	rreconcile.ProgressiveStatus(false, obj, meta.ProgressingReason, "generation kubernetes resources")

	productFolder := filepath.Join(dir, obj.Name)

	productDeployment, err := r.createProductDeployment(ctx, obj, *prodDesc, component, productFolder, cv, project)
	if err != nil {
		if errors.Is(err, unschedulableError) {
			status.MarkNotReady(r.EventRecorder, obj, v1alpha1.ProductPipelineSchedulingFailedReason, err.Error())

			// stop reconciling until fixed
			return ctrl.Result{}, nil
		}

		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.CreateProductPipelineFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create product deployment: %w", err)
	}

	// Note that we pass in the top level folder here.
	snapshotName, err := r.createSnapshot(ctx, obj, productDeployment, component, dir)
	if err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.CreateSnapshotFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create snapshot for product deployment: %w", err)
	}

	if project.Spec.Git.CommitTemplate == nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.CommitTemplateEmptyReason, "commit template in project cannot be empty")

		return ctrl.Result{}, fmt.Errorf("commit template in project cannot be empty")
	}

	repositoryRef := project.Status.RepositoryRef.Name
	if v := obj.Spec.RepositoryRef; v != nil {
		repositoryRef = v.Name
	}

	hashedVersion := r.hashComponentVersion(component.Version)

	sync := &gitv1alpha1.Sync{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-sync-" + hashedVersion,
			Namespace: obj.Namespace,
			Annotations: map[string]string{
				componentVersionAnnotationKey: component.Version,
				componentNameAnnotationKey:    component.Name,
			},
		},
		Spec: gitv1alpha1.SyncSpec{
			SnapshotRef: corev1.LocalObjectReference{
				Name: snapshotName,
			},
			RepositoryRef: meta.NamespacedObjectReference{
				Name:      repositoryRef,
				Namespace: r.MpasSystemNamespace,
			},
			Interval: metav1.Duration{
				Duration: 1 * time.Minute,
			},
			CommitTemplate: gitv1alpha1.CommitTemplate{
				Name:       project.Spec.Git.CommitTemplate.Name,
				Email:      project.Spec.Git.CommitTemplate.Email,
				Message:    project.Spec.Git.CommitTemplate.Message,
				BaseBranch: project.Spec.Git.DefaultBranch,
			},
			PullRequestTemplate: gitv1alpha1.PullRequestTemplate{
				Title: fmt.Sprintf("MPAS System Automated Pull Request for Product: %s", prodDesc.Name),
			},
			AutomaticPullRequestCreation: true,
			SubPath:                      "products/.",
		},
	}

	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, sync, func() error {
		if sync.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, sync, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to sync object: %w", err)
			}
		}

		return nil
	}); err != nil {
		status.MarkNotReady(r.EventRecorder, obj, v1alpha1.CreateSyncFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create sync request: %w", err)
	}

	obj.Status.LastReconciledVersion = component.Version
	status.MarkReady(r.EventRecorder, obj, "Applied version: %s", component.Version)

	return ctrl.Result{}, nil
}

func (r *ProductDeploymentGeneratorReconciler) createProductDeployment(
	ctx context.Context,
	obj *v1alpha1.ProductDeploymentGenerator,
	prodDesc v1alpha1.ProductDescription,
	component replicationv1.Component,
	dir string,
	cv ocm.ComponentVersionAccess,
	project *projectv1.Project,
) (*v1alpha1.ProductDeployment, error) {
	logger := log.FromContext(ctx)

	productDeployment := &v1alpha1.ProductDeployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       v1alpha1.ProductDeploymentKind,
			APIVersion: v1alpha1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name,
			Namespace: obj.Namespace,
		},
	}

	spec := v1alpha1.ProductDeploymentSpec{
		Component:          component,
		ServiceAccountName: obj.Spec.ServiceAccountName,
	}

	var readme []byte
	schemaFiles := make([]*cue.File, 0, len(prodDesc.Spec.Pipelines))

	for _, p := range prodDesc.Spec.Pipelines {
		pipe, file, err := r.createProductPipeline(ctx, prodDesc, p, cv)
		if err != nil {
			return nil, fmt.Errorf("failed to create product pipeline: %w", err)
		}

		spec.Pipelines = append(spec.Pipelines, pipe)

		if file == nil {
			continue
		}

		readme = append(readme, []byte(fmt.Sprintf("\n# Configuration instructions for %s\n\n", p.Name))...)

		parsed, err := r.increaseHeaders([]byte(file.Comments()))
		if err != nil {
			return nil, fmt.Errorf("failed to parse instructions: %w", err)
		}

		readme = append(readme, parsed...)
		schemaFiles = append(schemaFiles, file)
	}

	if len(schemaFiles) == 0 {
		return nil, fmt.Errorf("failed to create product pipeline, a schema file is required")
	}

	var (
		schema *cue.File
		err    error
	)
	if len(schemaFiles) > 1 {
		schema, err = schemaFiles[0].Unify(schemaFiles[1:])
		if err != nil {
			return nil, fmt.Errorf("failed to unify schemas: %w", err)
		}
	} else {
		schema = schemaFiles[0]
	}

	existingConfigFile, err := r.fetchExistingValues(ctx, obj.Name, project)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ignored values: %w", err)
	}

	config := schema
	var configNoPrivate *cue.File
	if existingConfigFile != nil {
		// we need the final config to hold private fields
		config, err = existingConfigFile.Merge(schema, true)
		if err != nil {
			return nil, fmt.Errorf("failed to merge existing config with schema: %w", err)
		}

		// but need a config without private fields for generated the config file
		if config.ContainsPrivateFields() {
			configNoPrivate, err = existingConfigFile.Merge(schema, false)
			if err != nil {
				return nil, fmt.Errorf("failed to merge existing config with schema: %w", err)
			}
		} else {
			configNoPrivate = config
		}

		err = configNoPrivate.Sanitize()
		if err != nil {
			return nil, fmt.Errorf("failed to sanitize config: %w", err)
		}
	}

	err = config.Sanitize()
	if err != nil {
		return nil, fmt.Errorf("failed to sanitize config: %w", err)
	}

	err = config.Validate(schema)
	if err != nil {
		return nil, fmt.Errorf("failed to validate configuration values file: %w", err)
	}

	if configNoPrivate == nil {
		configNoPrivate = config.CopyWithoutPrivateFields()
	}

	configBytes, err := configNoPrivate.Format()
	if err != nil {
		return nil, fmt.Errorf("failed to format config: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "config.cue"), configBytes, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write configuration values file: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), readme, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write readme file: %w", err)
	}

	// also create a configmap with the values in yaml format
	configYaml, err := config.Yaml()
	if err != nil {
		return nil, fmt.Errorf("failed to convert config to yaml: %w", err)
	}
	err = r.generateConfigMap(ctx, obj, string(configYaml))
	if err != nil {
		return nil, fmt.Errorf("failed to generate configmap: %w", err)
	}

	productDeployment.Spec = spec

	logger.Info("successfully generated product deployment", "productDeployment", klog.KObj(productDeployment))

	return productDeployment, nil
}

func (r *ProductDeploymentGeneratorReconciler) generateConfigMap(ctx context.Context, obj *v1alpha1.ProductDeploymentGenerator, config string) error {

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-values",
			Namespace: obj.Namespace,
		},
	}

	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, configMap, func() error {
		if configMap.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, configMap, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to sync object: %w", err)
			}
		}

		if configMap.Data == nil || len(configMap.Data) == 0 {
			configMap.Data = map[string]string{
				"values.yaml": config,
			}
		}

		if v, ok := configMap.Data["values.yaml"]; !ok || v != config {
			configMap.Data["values.yaml"] = config
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create configmap: %w", err)
	}
	return nil
}

// createProductPipeline takes a pipeline description and builds up all the Kubernetes objects that are needed
// for that resource.
func (r *ProductDeploymentGeneratorReconciler) createProductPipeline(
	ctx context.Context,
	description v1alpha1.ProductDescription,
	p v1alpha1.ProductDescriptionPipeline,
	cv ocm.ComponentVersionAccess,
) (v1alpha1.Pipeline, *cue.File, error) {
	var targetRole *v1alpha1.TargetRole

	if p.TargetRoleName == "" {
		return v1alpha1.Pipeline{}, nil, fmt.Errorf("pipeline '%s' cannot be scheduled: %w", p.Name, unschedulableError)
	}

	// if the list is empty, select one from the available targets.
	for _, role := range description.Spec.TargetRoles {
		if role.Name == p.TargetRoleName {
			targetRole = &role.TargetRole
			break
		}
	}

	if targetRole == nil {
		return v1alpha1.Pipeline{}, nil, fmt.Errorf("failed to find a target role with name %s: %w", p.TargetRoleName, unschedulableError)
	}

	var schemaFile *cue.File
	if p.Schema.Name != "" {
		content, err := r.OCMClient.GetResourceData(ctx, cv, p.Schema)
		if err != nil {
			return v1alpha1.Pipeline{}, nil, fmt.Errorf("failed to get schema data for %s: %w", p.Schema.Name, err)
		}

		// fetch the cue schema
		file, err := cue.New(p.Name, "", string(content))
		if err != nil {
			return v1alpha1.Pipeline{}, nil, fmt.Errorf("failed to create cue schema for %s: %w", p.Schema.Name, err)
		}

		schemaFile = file
	}

	return v1alpha1.Pipeline{
		Name:         p.Name,
		Localization: p.Localization,
		Configuration: v1alpha1.Configuration{
			Rules: p.Configuration.Rules,
		},
		Resource:   p.Source,
		TargetRole: *targetRole,
	}, schemaFile, nil
}

func (r *ProductDeploymentGeneratorReconciler) createSnapshot(ctx context.Context, obj *v1alpha1.ProductDeploymentGenerator, productDeployment *v1alpha1.ProductDeployment, component replicationv1.Component, dir string) (string, error) {
	serializer := json.NewSerializerWithOptions(json.DefaultMetaFactory, r.Scheme, r.Scheme, json.SerializerOptions{
		Yaml:   true,
		Pretty: true,
	})

	// Put the product-deployment into the deployment named folder.
	productDeploymentFile, err := os.Create(filepath.Join(dir, obj.Name, "product-deployment.yaml"))
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}

	defer productDeploymentFile.Close()

	if err := os.WriteFile(filepath.Join(dir, obj.Name, "kustomization.yaml"), []byte(kustomizationFile), 0o777); err != nil {
		return "", fmt.Errorf("failed to create kustomization file: %w", err)
	}

	if err := serializer.Encode(productDeployment, productDeploymentFile); err != nil {
		return "", fmt.Errorf("failed to encode product deployment: %w", err)
	}

	snapshotName, err := snapshot.GenerateSnapshotName(obj.Name)
	if err != nil {
		return "", fmt.Errorf("failed to generate a snapshot name: %w", err)
	}

	obj.Status.SnapshotName = snapshotName

	identity := ocmmetav1.Identity{
		ocmv1alpha1.ComponentNameKey:      component.Name,
		ocmv1alpha1.ComponentVersionKey:   component.Version,
		v1alpha1.ProductDeploymentNameKey: obj.Name,
	}

	digest, err := r.SnapshotWriter.Write(ctx, obj, dir, identity)
	if err != nil {
		return "", fmt.Errorf("failed to write to the cache: %w", err)
	}

	obj.Status.LatestSnapshotDigest = digest

	return snapshotName, nil
}

func (r *ProductDeploymentGeneratorReconciler) increaseHeaders(instructions []byte) ([]byte, error) {
	prioritizedTransformer := util.Prioritized(&headerTransformer{}, 0)

	gm := goldmark.New(
		goldmark.WithRenderer(markdown.NewRenderer()),
		goldmark.WithParserOptions(parser.WithASTTransformers(prioritizedTransformer)),
	)

	var buf bytes.Buffer
	if err := gm.Convert(instructions, &buf); err != nil {
		return nil, fmt.Errorf("failed to convert instructions: %w", err)
	}

	return buf.Bytes(), nil
}

type headerTransformer struct{}

func (t *headerTransformer) Transform(doc *ast.Document, reader text.Reader, pctx parser.Context) {
	_ = ast.Walk(doc, func(node ast.Node, enter bool) (ast.WalkStatus, error) {
		if enter {
			heading, ok := node.(*ast.Heading)
			if !ok {
				return ast.WalkContinue, nil
			}

			heading.Level++
		}

		return ast.WalkSkipChildren, nil
	})
}

// findGenerators looks for all the generator objects which have indexes on the ComponentVersion that is being
// fetched here. Specifically, the sourceKey's fields should match the field returned by ObjectKeyFromObject which
// will be the Name and Namespace of the ComponentSubscription. For which there are indexes set up in the Manager section.
func (r *ProductDeploymentGeneratorReconciler) findGenerators(ctx context.Context, o client.Object) []reconcile.Request {
	selectorTerm := client.ObjectKeyFromObject(o).String()

	generators := &v1alpha1.ProductDeploymentGeneratorList{}
	if err := r.List(context.TODO(), generators, &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(sourceKey, selectorTerm),
	}); err != nil {
		return []reconcile.Request{}
	}

	requests := make([]reconcile.Request, 0, len(generators.Items))

	for _, generator := range generators.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      generator.GetName(),
				Namespace: generator.GetNamespace(),
			},
		})
	}

	return requests
}

// hashComponentVersion provides a small chunk of a hexadecimal format for a version.
func (r *ProductDeploymentGeneratorReconciler) hashComponentVersion(version string) string {
	h := sha1.New()
	h.Write([]byte(version))

	return hex.EncodeToString(h.Sum(nil))[:8]
}

// fetchExistingValues gathers existing values.yaml values. If it doesn't exist it returns an empty file and no error.
func (r *ProductDeploymentGeneratorReconciler) fetchExistingValues(ctx context.Context, productName string, project *projectv1.Project) (*cue.File, error) {
	repoName, repoNamespace, err := FetchGitRepositoryFromProjectInventory(project)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch GitRepository from project: %w", err)
	}

	repo := &v1beta2.GitRepository{}
	if err := r.Get(ctx, types.NamespacedName{Name: repoName, Namespace: repoNamespace}, repo); err != nil {
		return nil, fmt.Errorf("failed to fetch git repository: %w", err)
	}

	if repo.Status.Artifact == nil {
		return nil, fmt.Errorf("git repository artifact is empty: %w", err)
	}

	path, dir, err := fetchFile(ctx, repo.Status.Artifact, productName)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to fetch existing values file: %w", err)
	}

	defer func() {
		if oerr := os.RemoveAll(dir); oerr != nil {
			err = errors.Join(err, oerr)
		}
	}()

	configFile, err := cue.New(productName, path, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create cue file: %w", err)
	}

	if err := configFile.Sanitize(); err != nil {
		return nil, fmt.Errorf("failed to sanitize existing values file: %w", err)
	}

	return configFile, nil
}
