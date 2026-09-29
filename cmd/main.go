package main

import (
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	confluentv1alpha1 "github.com/emanuelbesliu/confluent-identity-operator/api/v1alpha1"
	"github.com/emanuelbesliu/confluent-identity-operator/internal/confluent"
	"github.com/emanuelbesliu/confluent-identity-operator/internal/controller"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(confluentv1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr, probeAddr string
	var enableLeaderElection bool
	ctrl.SetLogger(zap.New(zap.UseDevMode(false)))

	metricsAddr = getEnv("METRICS_BIND_ADDRESS", ":8080")
	probeAddr = getEnv("HEALTH_PROBE_BIND_ADDRESS", ":8081")
	enableLeaderElection = getEnv("LEADER_ELECT", "true") == "true"

	cfg, err := loadConfig()
	if err != nil {
		setupLog.Error(err, "invalid operator configuration")
		os.Exit(1)
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "confluent-identity-operator.confluentoauth.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err := (&controller.ConfluentIdentityPoolReconciler{
		Client: mgr.GetClient(),
		Config: cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ConfluentIdentityPool")
		os.Exit(1)
	}
	if err := (&controller.ConfluentRoleBindingReconciler{
		Client: mgr.GetClient(),
		Config: cfg,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ConfluentRoleBinding")
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

	setupLog.Info("starting manager", "cluster", cfg.ClusterName, "provider", cfg.ProviderID, "dryRun", cfg.DryRun, "ownedPrefix", cfg.OwnedPrefix)
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

// loadConfig reads operator configuration from the environment.
func loadConfig() (controller.Config, error) {
	providerID := os.Getenv("CONFLUENT_IDENTITY_PROVIDER_ID")
	if providerID == "" {
		return controller.Config{}, fmt.Errorf("CONFLUENT_IDENTITY_PROVIDER_ID is required")
	}
	clusterName := os.Getenv("CLUSTER_NAME")
	if clusterName == "" {
		return controller.Config{}, fmt.Errorf("CLUSTER_NAME is required")
	}
	apiKey := os.Getenv("CONFLUENT_API_KEY")
	apiSecret := os.Getenv("CONFLUENT_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		return controller.Config{}, fmt.Errorf("CONFLUENT_API_KEY and CONFLUENT_API_SECRET are required")
	}

	cc := confluent.NewClient(os.Getenv("CONFLUENT_API_BASE"), apiKey, apiSecret)
	return controller.Config{
		Confluent:       cc,
		ProviderID:      providerID,
		DefaultAudience: os.Getenv("CONFLUENT_AUDIENCE"),
		ClusterName:     clusterName,
		CRNScope:        os.Getenv("CONFLUENT_CRN_SCOPE"),
		DryRun:          os.Getenv("DRY_RUN") == "true",
		OwnedPrefix:     os.Getenv("OWNED_PREFIX"),
	}, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
