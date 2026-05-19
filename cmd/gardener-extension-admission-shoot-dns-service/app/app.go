// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"context"
	"fmt"
	"os"

	controllercmd "github.com/gardener/gardener/extensions/pkg/controller/cmd"
	"github.com/gardener/gardener/extensions/pkg/util"
	webhookcmd "github.com/gardener/gardener/extensions/pkg/webhook/cmd"
	"github.com/gardener/gardener/pkg/apis/core/install"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	gardenerhealthz "github.com/gardener/gardener/pkg/healthz"
	"github.com/spf13/cobra"
	"k8s.io/client-go/rest"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"k8s.io/component-base/version"
	"k8s.io/component-base/version/verflag"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/cluster"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	admissioncmd "github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/cmd"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/validator"
	serviceinstall "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/install"
)

// AdmissionName is the name of the admission component.
const AdmissionName = "admission-shoot-dns-service"

var log = runtimelog.Log.WithName("gardener-extension-admission-shoot-dns-service")

// NewAdmissionCommand creates a new command for running an AWS admission webhook.
func NewAdmissionCommand(ctx context.Context) *cobra.Command {
	var (
		restOpts = &controllercmd.RESTOptions{}
		mgrOpts  = &controllercmd.ManagerOptions{
			LeaderElection:          true,
			LeaderElectionID:        controllercmd.LeaderElectionNameID(AdmissionName),
			LeaderElectionNamespace: os.Getenv("LEADER_ELECTION_NAMESPACE"),
			WebhookServerPort:       443,
			HealthBindAddress:       ":8081",
			WebhookCertDir:          "/tmp/admission-shoot-dns-service-cert",
		}
		// options for the webhook server
		webhookServerOptions = &webhookcmd.ServerOptions{
			Namespace: os.Getenv("WEBHOOK_CONFIG_NAMESPACE"),
		}
		webhookSwitches = admissioncmd.GardenWebhookSwitchOptions()
		webhookOptions  = webhookcmd.NewAddToManagerOptions(
			AdmissionName,
			"",
			nil,
			nil,
			webhookServerOptions,
			webhookSwitches,
		)

		admissionOpts = &admissioncmd.ConfigOptions{}

		aggOption = controllercmd.NewOptionAggregator(
			restOpts,
			mgrOpts,
			admissionOpts,
			webhookOptions,
		)
	)

	cmd := &cobra.Command{
		Use: "admission webhooks of shoot-dns-service",
		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			log.Info("Starting "+AdmissionName, "version", version.Get())

			if gardenKubeconfig := os.Getenv("GARDEN_KUBECONFIG"); gardenKubeconfig != "" {
				log.Info("Getting rest config for garden from GARDEN_KUBECONFIG", "path", gardenKubeconfig)
				restOpts.Kubeconfig = gardenKubeconfig
			}

			if err := aggOption.Complete(); err != nil {
				runtimelog.Log.Error(err, "Error completing options")
				os.Exit(1)
			}

			util.ApplyClientConnectionConfigurationToRESTConfig(&componentbaseconfigv1alpha1.ClientConnectionConfiguration{
				QPS:   100.0,
				Burst: 130,
			}, restOpts.Completed().Config)

			managerOptions := mgrOpts.Completed().Options()

			if admissionConfig := admissionOpts.Completed(); admissionConfig != nil {
				validator.DefaultAddOptions.GCPWorkloadIdentityConfig = *admissionConfig
			} else {
				return fmt.Errorf("could not complete admission options")
			}

			log.Info("Configuring source cluster option")
			inClusterConfig, err := rest.InClusterConfig()
			if err != nil {
				return fmt.Errorf("could not get in-cluster config: %w", err)
			}
			managerOptions.LeaderElectionConfig = inClusterConfig

			mgr, err := manager.New(restOpts.Completed().Config, managerOptions)
			if err != nil {
				runtimelog.Log.Error(err, "Could not instantiate manager")
				os.Exit(1)
			}

			install.Install(mgr.GetScheme())

			if err := serviceinstall.AddToScheme(mgr.GetScheme()); err != nil {
				runtimelog.Log.Error(err, "Could not update manager scheme")
				os.Exit(1)
			}
			if err := securityv1alpha1.AddToScheme(mgr.GetScheme()); err != nil {
				runtimelog.Log.Error(err, "Could not update manager scheme with security.gardener.cloud/v1alpha1 API version")
				os.Exit(1)
			}

			sourceCluster, err := cluster.New(inClusterConfig, func(opts *cluster.Options) {
				opts.Logger = log
				opts.Cache.DefaultNamespaces = map[string]cache.Config{v1beta1constants.GardenNamespace: {}}
			})
			if err != nil {
				return err
			}

			if err := mgr.AddReadyzCheck("source-informer-sync", gardenerhealthz.NewCacheSyncHealthz(sourceCluster.GetCache())); err != nil {
				return err
			}

			if err = mgr.Add(sourceCluster); err != nil {
				return err
			}

			log.Info("Setting up webhook server")
			if _, err := webhookOptions.Completed().AddToManager(ctx, mgr, sourceCluster); err != nil {
				return err
			}

			if err := mgr.AddReadyzCheck("informer-sync", gardenerhealthz.NewCacheSyncHealthz(mgr.GetCache())); err != nil {
				return fmt.Errorf("could not add readycheck for informers: %w", err)
			}

			if err := mgr.AddHealthzCheck("ping", healthz.Ping); err != nil {
				return fmt.Errorf("could not add healthcheck: %w", err)
			}

			if err := mgr.AddReadyzCheck("webhook-server", mgr.GetWebhookServer().StartedChecker()); err != nil {
				return fmt.Errorf("could not add readycheck of webhook to manager: %w", err)
			}

			return mgr.Start(ctx)
		},
	}

	aggOption.AddFlags(cmd.Flags())

	return cmd
}
