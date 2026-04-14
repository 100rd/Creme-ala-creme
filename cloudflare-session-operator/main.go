package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Creme-ala-creme/cloudflare-session-operator/api/v1alpha1"
	"github.com/Creme-ala-creme/cloudflare-session-operator/controllers"
	"github.com/Creme-ala-creme/cloudflare-session-operator/pkg/cloudflare"
	"github.com/go-logr/stdr"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
}

// validateCredentials checks that required Cloudflare credentials are set.
// Returns nil if dry-run mode is enabled or credentials are present.
func validateCredentials() error {
	if strings.EqualFold(os.Getenv("CLOUDFLARE_DRY_RUN"), "true") {
		return nil
	}
	if os.Getenv("CLOUDFLARE_ACCOUNT_ID") == "" {
		return fmt.Errorf("CLOUDFLARE_ACCOUNT_ID is required (set CLOUDFLARE_DRY_RUN=true to skip)")
	}
	if os.Getenv("CLOUDFLARE_API_TOKEN") == "" {
		return fmt.Errorf("CLOUDFLARE_API_TOKEN is required (set CLOUDFLARE_DRY_RUN=true to skip)")
	}
	return nil
}

// resolveWatchNamespace determines which namespace the operator should watch.
// Priority: WATCH_NAMESPACE env > POD_NAMESPACE env > empty (all namespaces).
func resolveWatchNamespace() string {
	if ns := os.Getenv("WATCH_NAMESPACE"); ns != "" {
		return ns
	}
	return os.Getenv("POD_NAMESPACE")
}

func main() {
	var metricsAddr string
	var probeAddr string
	var enableLeaderElection bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false, "Enable leader election for controller manager.")
	flag.Parse()

	logger := stdr.New(log.New(os.Stdout, "", log.LstdFlags))
	ctrllog.SetLogger(logger)

	// Issue #3: Fail-fast if Cloudflare credentials are missing.
	if err := validateCredentials(); err != nil {
		setupLog.Error(err, "credential validation failed")
		os.Exit(1)
	}

	// Issue #8: Namespace-scoped cache to restrict the operator's watch scope.
	cacheOpts := cache.Options{
		SyncPeriod: func() *time.Duration {
			d := 5 * time.Minute
			return &d
		}(),
	}
	if watchNS := resolveWatchNamespace(); watchNS != "" {
		cacheOpts.DefaultNamespaces = map[string]cache.Config{
			watchNS: {},
		}
		setupLog.Info("restricting watch to namespace", "namespace", watchNS)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: metricsAddr,
		},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "sessionbinding.cloudflare.example",
		Cache:                  cacheOpts,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	cfClient := cloudflare.NewClientFromEnv()

	if err = (&controllers.SessionBindingReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		CFClient: cfClient,
		Recorder: mgr.GetEventRecorderFor("sessionbinding-controller"),
		Clock:    controllers.RealClock{},
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "SessionBinding")
		os.Exit(1)
	}

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
