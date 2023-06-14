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
	"github.com/fluxcd/source-controller/api/v1beta2"
	goyaml "github.com/goccy/go-yaml"
	goyamlast "github.com/goccy/go-yaml/ast"
	goyamlparser "github.com/goccy/go-yaml/parser"
	projectv1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"
	markdown "github.com/teekennedy/goldmark-markdown"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer/json"
	"k8s.io/apimachinery/pkg/types"
	kyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/klog/v2"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	gitv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	ocmv1alpha1 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	ocmconfig "github.com/open-component-model/ocm-controller/pkg/configdata"
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
}

// SetupWithManager sets up the controller with the Manager.
func (r *ProductDeploymentGeneratorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	const (
		sourceKey = ".metadata.source"
	)

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
			&source.Kind{Type: &replicationv1.ComponentSubscription{}},
			handler.EnqueueRequestsFromMapFunc(r.findGenerators(sourceKey)),
			builder.WithPredicates(ComponentSubscriptionVersionChangedPredicate{})).
		Complete(r)
}

//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=productdeploymentgenerators/finalizers,verbs=update
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=projects,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mpas.ocm.software,resources=targets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentsubscriptions,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=componentversions;componentdescriptors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=localizations;configurations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=delivery.ocm.software,resources=fluxdeployers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=gitrepositories;ocirepositories;buckets;ocirepositories,verbs=create;update;patch;delete;get;list;watch
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

	patchHelper := patch.NewSerialPatcher(obj, r.Client)

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

	if obj.Status.LastReconciledVersion != "" {
		lastReconciledGeneratorVersion, err := semver.NewVersion(obj.Status.LastReconciledVersion)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to parse last reconciled version: %w", err)
		}

		lastReconciledSubscription, err := semver.NewVersion(subscription.Status.LastAppliedVersion)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to parse last reconciled version: %w", err)
		}

		if lastReconciledSubscription.Equal(lastReconciledGeneratorVersion) || lastReconciledSubscription.LessThan(lastReconciledGeneratorVersion) {
			logger.Info("data generator version is greater than or equal to last reconciled subscription version", "generator", lastReconciledGeneratorVersion.Original(), "subscription", lastReconciledSubscription.Original())
			return ctrl.Result{}, nil
		}
	}

	project, err := GetProjectInNamespace(ctx, r.Client, r.MpasSystemNamespace)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProjectInNamespaceGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to find the project in the namespace: %w", err)
	}

	if !conditions.IsReady(project) {
		logger.Info("project not ready yet")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	if project.Status.RepositoryRef == nil {
		logger.Info("no repository information is provided yet")

		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

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
	defer cv.Close()

	logger.Info("retrieved component version, fetching ProductDescription resource", "component", cv.GetName())

	content, err := r.OCMClient.GetProductDescription(ctx, octx, cv)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to get product description data: %w", err)
	}

	prodDesc := &v1alpha1.ProductDescription{}
	if err := kyaml.Unmarshal(content, prodDesc); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductDescriptionGetFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to unmarshal product description: %w", err)
	}

	logger.Info("fetched product description", "description", klog.KObj(prodDesc))

	dir, err := os.MkdirTemp("", "product-deployment")
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create temp folder: %w", err)
	}

	defer os.RemoveAll(dir)

	// Create top-level product folder.
	if err := os.Mkdir(filepath.Join(dir, obj.Name), 0o777); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to create product folder: %w", err)
	}

	productFolder := filepath.Join(dir, obj.Name)

	validationRules := make([]v1alpha1.ValidationData, 0)

	productDeployment, err := r.createProductDeployment(ctx, obj, *prodDesc, component, productFolder, cv, &validationRules, project)
	if err != nil {
		if errors.Is(err, unschedulableError) {
			conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.ProductPipelineSchedulingFailedReason, err.Error())

			// stop reconciling until fixed
			return ctrl.Result{}, nil
		}

		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.CreateProductPipelineFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create product deployment: %w", err)
	}

	// Note that we pass in the top level folder here.
	snapshotName, err := r.createSnapshot(ctx, obj, productDeployment, component, dir)
	if err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.CreateSnapshotFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create snapshot for product deployment: %w", err)
	}

	if project.Spec.Git.CommitTemplate == nil {
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
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.CreateSyncFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create sync request: %w", err)
	}

	// Create the Validation Object.
	validation := &v1alpha1.Validation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.Name + "-validation-" + hashedVersion,
			Namespace: obj.Namespace,
			Annotations: map[string]string{
				componentVersionAnnotationKey: component.Version,
				componentNameAnnotationKey:    component.Name,
			},
		},
		Spec: v1alpha1.ValidationSpec{
			ValidationRules:    validationRules,
			ServiceAccountName: obj.Spec.ServiceAccountName,
			Interval:           metav1.Duration{Duration: 10 * time.Second},
			SyncRef: meta.NamespacedObjectReference{
				Name:      sync.Name,
				Namespace: sync.Namespace,
			},
		},
	}

	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, validation, func() error {
		if validation.ObjectMeta.CreationTimestamp.IsZero() {
			if err := controllerutil.SetOwnerReference(obj, validation, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner to validation object: %w", err)
			}
		}

		return nil
	}); err != nil {
		conditions.MarkFalse(obj, meta.ReadyCondition, v1alpha1.CreateValidationFailedReason, err.Error())

		return ctrl.Result{}, fmt.Errorf("failed to create validation request: %w", err)
	}

	obj.Status.LastReconciledVersion = component.Version

	return ctrl.Result{}, nil
}

func (r *ProductDeploymentGeneratorReconciler) createProductDeployment(
	ctx context.Context,
	obj *v1alpha1.ProductDeploymentGenerator,
	prodDesc v1alpha1.ProductDescription,
	component replicationv1.Component,
	dir string,
	cv ocm.ComponentVersionAccess,
	validationRules *[]v1alpha1.ValidationData,
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

	values := make(map[string]map[string]any)
	var readme []byte

	for _, p := range prodDesc.Spec.Pipelines {
		pipe, instructions, err := r.createProductPipeline(ctx, prodDesc, p, cv, values)
		if err != nil {
			return nil, fmt.Errorf("failed to create product pipeline: %w", err)
		}

		spec.Pipelines = append(spec.Pipelines, pipe)

		readme = append(readme, []byte(fmt.Sprintf("\n# Configuration instructions for %s\n\n", p.Name))...)

		parsed, err := r.increaseHeaders(instructions)
		if err != nil {
			return nil, fmt.Errorf("failed to parse instructions: %w", err)
		}

		readme = append(readme, parsed...)

		// fetch the validation rules
		data, err := r.OCMClient.GetResourceData(ctx, cv, p.Validation)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch validation data: %w", err)
		}

		*validationRules = append(*validationRules, v1alpha1.ValidationData{
			Name: p.Name,
			Data: data,
		})
	}

	// this is EXPLICITLY using normal yaml.
	// goyaml, for some reason, parses ints as floats.
	defaultConfig, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default values: %w", err)
	}

	existingValuesFile, err := r.fetchExistingValues(ctx, obj.Name, project)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ignored values: %w", err)
	}

	valuesYaml := defaultConfig
	if existingValuesFile != nil {
		parsedValues, err := goyamlparser.ParseBytes(defaultConfig, goyamlparser.ParseComments)
		if err != nil {
			return nil, fmt.Errorf("failed to parse existing values file: %w", err)
		}

		v := &Visitor{
			replace: parsedValues,
		}

		for _, doc := range existingValuesFile.Docs {
			goyamlast.Walk(v, doc)
		}

		if v.err != nil {
			return nil, fmt.Errorf("failed to update values file: %w", v.err)
		}

		// A silly little fix for when the generated content doesn't have a linebreak.
		content := []byte(v.replace.String())
		if !bytes.HasSuffix(content, []byte("\n")) {
			content = append(content, '\n')
		}

		valuesYaml = content
	}

	if err := os.WriteFile(filepath.Join(dir, "values.yaml"), valuesYaml, 0o777); err != nil {
		return nil, fmt.Errorf("failed to write configuration values file: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), readme, 0o777); err != nil {
		return nil, fmt.Errorf("failed to write readme file: %w", err)
	}

	productDeployment.Spec = spec

	logger.Info("successfully generated product deployment", "productDeployment", klog.KObj(productDeployment))

	return productDeployment, nil
}

// createProductPipeline takes a pipeline description and builds up all the Kubernetes objects that are needed
// for that resource.
func (r *ProductDeploymentGeneratorReconciler) createProductPipeline(
	ctx context.Context,
	description v1alpha1.ProductDescription,
	p v1alpha1.ProductDescriptionPipeline,
	cv ocm.ComponentVersionAccess,
	values map[string]map[string]any,
) (v1alpha1.Pipeline, []byte, error) {
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

	// fetch values and create values.yaml file in dir with pipeline.Name-values.yaml
	if p.Configuration.Rules.Name != "" {
		content, err := r.OCMClient.GetResourceData(ctx, cv, p.Configuration.Rules)
		if err != nil {
			return v1alpha1.Pipeline{}, nil, fmt.Errorf("failed to get resource data for %s: %w", p.Configuration.Rules.Name, err)
		}

		configData := &ocmconfig.ConfigData{}
		if err := kyaml.Unmarshal(content, configData); err != nil {
			return v1alpha1.Pipeline{}, nil, fmt.Errorf("failed to get unmarshal config data: %w", err)
		}

		values[p.Name] = configData.Configuration.Defaults
	}

	// add readme
	instructions, err := r.OCMClient.GetResourceData(ctx, cv, p.Configuration.Readme)
	if err != nil {
		return v1alpha1.Pipeline{}, nil, fmt.Errorf("failed to get readme data for %s: %w", p.Configuration.Readme.Name, err)
	}

	return v1alpha1.Pipeline{
		Name:         p.Name,
		Localization: p.Localization,
		Configuration: v1alpha1.Configuration{
			Rules: p.Configuration.Rules,
		},
		Resource:   p.Source,
		TargetRole: *targetRole,
		Validation: p.Validation,
	}, instructions, nil
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
func (r *ProductDeploymentGeneratorReconciler) findGenerators(sourceKey string) handler.MapFunc {
	return func(object client.Object) []reconcile.Request {
		selectorTerm := client.ObjectKeyFromObject(object).String()

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
}

// hashComponentVersion provides a small chunk of a hexadecimal format for a version.
func (r *ProductDeploymentGeneratorReconciler) hashComponentVersion(version string) string {
	h := sha1.New()
	h.Write([]byte(version))

	return hex.EncodeToString(h.Sum(nil))[:8]
}

type Visitor struct {
	replace *goyamlast.File
	err     error
}

func (v *Visitor) Visit(node goyamlast.Node) goyamlast.Visitor {
	// quit early if there was an error
	if v.err != nil {
		return v
	}

	comment := node.GetComment()
	if comment == nil {
		return v
	}

	path, err := goyaml.PathString(node.GetPath())
	if err != nil {
		v.err = fmt.Errorf("failed to parse path string: %w", err)
		return v
	}

	replaceNode, err := path.FilterFile(v.replace)
	if err != nil {
		v.err = fmt.Errorf("failed to filter incoming values with path %s: %w", node.GetPath(), err)
		return v
	}

	if err := replaceNode.SetComment(comment); err != nil {
		return v
	}
	tk := replaceNode.GetToken()
	tk.Value = node.GetToken().Value

	return v
}

func (r *ProductDeploymentGeneratorReconciler) fetchExistingValues(ctx context.Context, productName string, project *projectv1.Project) (*goyamlast.File, error) {
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

	content, err := FetchValuesFileContent(ctx, productName, repo.Status.Artifact)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}

		return nil, fmt.Errorf("failed to fetch existing values file: %w", err)
	}

	parsedConfig, err := goyamlparser.ParseBytes(content, goyamlparser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("failed to parse existing values file: %w", err)
	}

	return parsedConfig, nil
}
