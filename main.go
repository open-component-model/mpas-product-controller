// SPDX-FileCopyrightText: 2022 SAP SE or an SAP affiliate company and Open Component Model contributors.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"flag"
	"os"

	"github.com/fluxcd/source-controller/api/v1beta2"
	gitv1alpha1 "github.com/open-component-model/git-controller/apis/delivery/v1alpha1"
	gitmpasv1alpha1 "github.com/open-component-model/git-controller/apis/mpas/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/pkg/validators/github"
	v1alpha12 "github.com/open-component-model/ocm-controller/api/v1alpha1"
	"github.com/open-component-model/ocm-controller/pkg/oci"
	"github.com/open-component-model/ocm-controller/pkg/snapshot"
	replicationv1 "github.com/open-component-model/replication-controller/api/v1alpha1"

	"github.com/open-component-model/mpas-product-controller/pkg/deployers/kubernetes"
	"github.com/open-component-model/mpas-product-controller/pkg/ocm"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	mpasprojv1alpha1 "github.com/open-component-model/mpas-project-controller/api/v1alpha1"

	mpasv1alpha1 "github.com/open-component-model/mpas-product-controller/api/v1alpha1"
	"github.com/open-component-model/mpas-product-controller/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme                     = runtime.NewScheme()
	setupLog                   = ctrl.Log.WithName("setup")
	defaultMpasSystemNamespace = "mpas-system"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(replicationv1.AddToScheme(scheme))
	utilruntime.Must(mpasv1alpha1.AddToScheme(scheme))
	utilruntime.Must(mpasprojv1alpha1.AddToScheme(scheme))
	utilruntime.Must(v1alpha12.AddToScheme(scheme))
	utilruntime.Must(gitv1alpha1.AddToScheme(scheme))
	utilruntime.Must(v1beta2.AddToScheme(scheme))
	utilruntime.Must(gitmpasv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	var ociRegistryAddr string
	var mpasSystemNamespace string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&ociRegistryAddr, "oci-registry-addr", ":5000", "The address of the OCI registry.")
	flag.StringVar(&mpasSystemNamespace, "mpas-system-namespace", defaultMpasSystemNamespace, "The namespace in which this controller is running in. This namespace is used to locate Project objects.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b3469b71.ocm.software",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	cache := oci.NewClient(ociRegistryAddr)
	snapshotWriter := snapshot.NewOCIWriter(mgr.GetClient(), cache, mgr.GetScheme())
	ocmClient := ocm.NewClient(mgr.GetClient())
	if err = (&controllers.ProductDeploymentGeneratorReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		OCMClient:           ocmClient,
		SnapshotWriter:      snapshotWriter,
		MpasSystemNamespace: mpasSystemNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProductDeploymentGenerator")
		os.Exit(1)
	}
	if err = (&controllers.ProductDeploymentReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProductDeployment")
		os.Exit(1)
	}
	if err = (&controllers.ProductDeploymentPipelineReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		MpasSystemNamespace: mpasSystemNamespace,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ProductDeploymentPipeline")
		os.Exit(1)
	}

	kubeDeployer := kubernetes.NewDeployer(mgr.GetClient(), mgr.GetScheme(), nil)
	if err = (&controllers.ProductDeploymentPipelineScheduler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		MpasSystemNamespace: mpasSystemNamespace,
		Deployer:            kubeDeployer,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create scheduler", "scheduler", "ProductDeploymentScheduler")
		os.Exit(1)
	}

	githubValidator := github.NewValidator(mgr.GetClient(), nil)
	if err = (&controllers.ValidationReconciler{
		Client:              mgr.GetClient(),
		Scheme:              mgr.GetScheme(),
		MpasSystemNamespace: mpasSystemNamespace,
		Validator:           githubValidator,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Validation")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
