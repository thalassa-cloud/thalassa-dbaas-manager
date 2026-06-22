/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"crypto/tls"
	"flag"
	"os"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"github.com/spf13/viper"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	"github.com/thalassa-cloud/client-go/dbaas"
	"github.com/thalassa-cloud/client-go/iaas"

	dbaasv1 "github.com/thalassa-cloud/thalassa-dbaas-manager/api/v1"
	"github.com/thalassa-cloud/thalassa-dbaas-manager/internal/controller"
	"github.com/thalassa-cloud/thalassa-dbaas-manager/internal/thalassaclient"
	// +kubebuilder:scaffold:imports
)

const thalassaClientHint = "unable to create Thalassa client; set --organisation and one of: " +
	"--thalassa-service-account-id (OIDC token exchange; uses in-cluster SA token path by default), " +
	"--thalassa-token-file or --thalassa-token, or --thalassa-client-id with " +
	"--thalassa-client-secret-file or --thalassa-client-secret"

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(dbaasv1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

// nolint:gocyclo
func main() {
	var metricsAddr string
	var metricsCertPath, metricsCertName, metricsCertKey string
	var webhookCertPath, webhookCertName, webhookCertKey string
	var enableLeaderElection bool
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var tlsOpts []func(*tls.Config)
	flag.StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. "+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.BoolVar(&secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(&metricsCertPath, "metrics-cert-path", "",
		"The directory that contains the metrics server certificate.")
	flag.StringVar(&metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(&metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")
	flag.BoolVar(&enableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers")

	// Thalassa flags (bound to viper after Parse so iaas client can read them).
	// Prefer file flags for secrets (mounted volumes).
	var (
		thalassaToken, thalassaTokenFile, thalassaClientID         string
		thalassaClientSecret, thalassaClientSecretFile             string
		thalassaURL, thalassaRegion, organisation, thalassaProject string
	)
	var thalassaServiceAccountID, thalassaSubjectTokenFile, thalassaSubjectToken string
	var thalassaOIDCTokenURL, thalassaAccessTokenLifetime string
	var thalassaInsecure bool
	flag.StringVar(&thalassaToken, "thalassa-token", "",
		"Thalassa personal access token (prefer --thalassa-token-file for production)")
	flag.StringVar(&thalassaTokenFile, "thalassa-token-file", "",
		"Path to file containing Thalassa personal access token (e.g. mounted Kubernetes secret)")
	flag.StringVar(&thalassaClientID, "thalassa-client-id", "", "Thalassa Cloud client ID (OAuth2 client credentials)")
	flag.StringVar(&thalassaClientSecret, "thalassa-client-secret", "",
		"Thalassa client secret (prefer --thalassa-client-secret-file for production)")
	flag.StringVar(&thalassaClientSecretFile, "thalassa-client-secret-file", "",
		"Path to file containing OAuth2 client secret")
	flag.BoolVar(&thalassaInsecure, "thalassa-insecure", false, "Use insecure connection to Thalassa Cloud API")
	flag.StringVar(&thalassaURL, "thalassa-url", "https://api.thalassa.cloud/", "Thalassa Cloud API URL")
	flag.StringVar(&thalassaRegion, "thalassa-region", "", "Thalassa Cloud region slug or identity")
	flag.StringVar(&thalassaProject, "thalassa-project", "", "Optional Thalassa project scope")
	flag.StringVar(&organisation, "organisation", "", "Thalassa Cloud organisation ID or Slug")
	flag.StringVar(&thalassaServiceAccountID, "thalassa-service-account-id", "",
		"Thalassa service account ID for OIDC token exchange (federated workload identity); "+
			"uses Kubernetes SA token file by default")
	flag.StringVar(&thalassaSubjectTokenFile, "thalassa-subject-token-file", "",
		"Path to subject JWT for token exchange (default: in-cluster service account token path when unset)")
	flag.StringVar(&thalassaSubjectToken, "thalassa-subject-token", "",
		"Inline subject JWT for token exchange (alternative to subject token file)")
	flag.StringVar(&thalassaOIDCTokenURL, "thalassa-oidc-token-url", "",
		"OIDC token endpoint (default: {thalassa-url}/oidc/token)")
	flag.StringVar(&thalassaAccessTokenLifetime, "thalassa-access-token-lifetime", "",
		"Optional exchanged access token lifetime (e.g. 39600s)")

	// DBaaS manager: identity of the Kubernetes cluster (Thalassa Cloud) and default subnet for provisioning DB clusters
	var clusterID, defaultSubnetID, defaultSecurityGroupID, defaultDbObjectStoreID, defaultRegion string
	flag.StringVar(&clusterID, "cluster-id", "", "Thalassa Cloud Kubernetes cluster identity (required for DBaaS provisioning)")
	flag.StringVar(&defaultSubnetID, "default-subnet-id", "", "Default Thalassa subnet identity when spec.subnet does not resolve a subnet")
	flag.StringVar(&defaultSecurityGroupID, "default-security-group-id", "", "Optional default Thalassa security group when spec.securityGroups is empty")
	flag.StringVar(&defaultDbObjectStoreID, "default-dbobjectstore-id", "", "Optional default Thalassa DbObjectStore identity when spec.dbObjectStore does not resolve one")
	flag.StringVar(&defaultRegion, "default-region", "", "Default Thalassa region (identity or slug) when DbObjectStore spec.region is empty; subnet discovery is tried if this is empty")
	if clusterID == "" {
		clusterID = os.Getenv("CLUSTER_ID")
	}
	if defaultSubnetID == "" {
		defaultSubnetID = os.Getenv("DEFAULT_SUBNET_ID")
	}
	if defaultSecurityGroupID == "" {
		defaultSecurityGroupID = os.Getenv("DEFAULT_SECURITY_GROUP_ID")
	}
	if defaultDbObjectStoreID == "" {
		defaultDbObjectStoreID = os.Getenv("DEFAULT_DBOBJECTSTORE_ID")
	}
	if defaultRegion == "" {
		defaultRegion = os.Getenv("REGION")
	}

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	// Thalassa settings for internal/iaas (flags only; do not use THALASSA_* env for controller config)
	if thalassaToken != "" {
		viper.Set("thalassa-token", thalassaToken)
	}
	if thalassaTokenFile != "" {
		viper.Set("thalassa-token-file", thalassaTokenFile)
	}
	if thalassaClientID != "" {
		viper.Set("thalassa-client-id", thalassaClientID)
	}
	if thalassaClientSecret != "" {
		viper.Set("thalassa-client-secret", thalassaClientSecret)
	}
	if thalassaClientSecretFile != "" {
		viper.Set("thalassa-client-secret-file", thalassaClientSecretFile)
	}
	viper.Set("thalassa-insecure", thalassaInsecure)
	viper.Set("thalassa-url", thalassaURL)
	if thalassaRegion != "" {
		viper.Set("thalassa-region", thalassaRegion)
	}
	if thalassaProject != "" {
		viper.Set("thalassa-project", thalassaProject)
	}
	if organisation != "" {
		viper.Set("organisation", organisation)
	}
	if thalassaServiceAccountID != "" {
		viper.Set("thalassa-service-account-id", thalassaServiceAccountID)
	}
	if thalassaSubjectTokenFile != "" {
		viper.Set("thalassa-subject-token-file", thalassaSubjectTokenFile)
	}
	if thalassaSubjectToken != "" {
		viper.Set("thalassa-subject-token", thalassaSubjectToken)
	}
	if thalassaOIDCTokenURL != "" {
		viper.Set("thalassa-oidc-token-url", thalassaOIDCTokenURL)
	}
	if thalassaAccessTokenLifetime != "" {
		viper.Set("thalassa-access-token-lifetime", thalassaAccessTokenLifetime)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if defaultSubnetID == "" {
		setupLog.Error(nil, "default-subnet-id is required")
		os.Exit(1)
	}

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Initial webhook TLS options
	webhookTLSOpts := tlsOpts
	webhookServerOptions := webhook.Options{
		TLSOpts: webhookTLSOpts,
	}

	if len(webhookCertPath) > 0 {
		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

		webhookServerOptions.CertDir = webhookCertPath
		webhookServerOptions.CertName = webhookCertName
		webhookServerOptions.KeyName = webhookCertKey
	}

	webhookServer := webhook.NewServer(webhookServerOptions)

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		TLSOpts:       tlsOpts,
	}

	if secureMetrics {
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		metricsServerOptions.CertDir = metricsCertPath
		metricsServerOptions.CertName = metricsCertName
		metricsServerOptions.KeyName = metricsCertKey
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
		WebhookServer:          webhookServer,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "thalassa-dbaas-manager.controllers.thalassa.cloud",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	thalassaClient, err := thalassaclient.NewClientFromEnv()
	if err != nil {
		setupLog.Error(err, thalassaClientHint)
		os.Exit(1)
	}
	iaasClient, err := iaas.New(thalassaClient)
	if err != nil {
		setupLog.Error(err, "unable to create Thalassa IaaS client (used for volume types and subnet)")
		os.Exit(1)
	}
	dbaasClient, err := dbaas.New(thalassaClient)
	if err != nil {
		setupLog.Error(err, "unable to create Thalassa DBaaS client")
		os.Exit(1)
	}

	// try and verify the default subnet
	if defaultSubnetID != "" {
		_, err := iaasClient.GetSubnet(context.Background(), defaultSubnetID)
		if err != nil {
			setupLog.Error(err, "unable to get default subnet")
			os.Exit(1)
		}
	}

	if err := (&controller.PostgresClusterReconciler{
		Client:                 mgr.GetClient(),
		Scheme:                 mgr.GetScheme(),
		DbaasClient:            dbaasClient,
		IaaSClient:             iaasClient,
		ClusterID:              clusterID,
		DefaultSubnetID:        defaultSubnetID,
		DefaultSecurityGroupID: defaultSecurityGroupID,
		DefaultDbObjectStoreID: defaultDbObjectStoreID,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgresCluster")
		os.Exit(1)
	}
	if err := (&controller.PostgresRoleReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		DbaasClient: dbaasClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgresRole")
		os.Exit(1)
	}
	if err := (&controller.PostgresDatabaseReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		DbaasClient: dbaasClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgresDatabase")
		os.Exit(1)
	}
	if err := (&controller.DbObjectStoreReconciler{
		Client:          mgr.GetClient(),
		Scheme:          mgr.GetScheme(),
		DbaasClient:     dbaasClient,
		IaaSClient:      iaasClient,
		DefaultRegion:   defaultRegion,
		DefaultSubnetID: defaultSubnetID,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DbObjectStore")
		os.Exit(1)
	}
	if err := (&controller.PostgresGrantReconciler{
		Client:      mgr.GetClient(),
		Scheme:      mgr.GetScheme(),
		DbaasClient: dbaasClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "PostgresGrant")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

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
