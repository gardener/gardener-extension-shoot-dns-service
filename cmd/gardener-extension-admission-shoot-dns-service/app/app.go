// Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package app

import (
	"context"
	"fmt"

	controllercmd "github.com/gardener/gardener/extensions/pkg/controller/cmd"
	"github.com/gardener/gardener/extensions/pkg/util"
	"github.com/gardener/gardener/extensions/pkg/util/index"
	"github.com/gardener/gardener/pkg/apis/core/install"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/spf13/cobra"
	componentbaseconfig "k8s.io/component-base/config"
	"k8s.io/component-base/version/verflag"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var log = logf.Log.WithName("gardener-extensions-validator-vsphere")

// NewValidatorCommand creates a new command for running an Openstack validator.
func NewValidatorCommand(ctx context.Context) *cobra.Command {
	var (
		restOpts = &controllercmd.RESTOptions{}
		mgrOpts  = &controllercmd.ManagerOptions{
			WebhookServerPort: 443,
		}

		aggOption = controllercmd.NewOptionAggregator(
			restOpts,
			mgrOpts,
		)
	)

	cmd := &cobra.Command{
		Use: "validator-shoot-dns-service",

		RunE: func(cmd *cobra.Command, args []string) error {
			verflag.PrintAndExitIfRequested()

			if err := aggOption.Complete(); err != nil {
				return errors.Wrap(err, "Error completing options")
			}

			util.ApplyClientConnectionConfigurationToRESTConfig(&componentbaseconfig.ClientConnectionConfiguration{
				QPS:   100.0,
				Burst: 130,
			}, restOpts.Completed().Config)

			mgr, err := manager.New(restOpts.Completed().Config, mgrOpts.Completed().Options())
			if err != nil {
				return errors.Wrap(err, "Could not instantiate manager")
			}

			install.Install(mgr.GetScheme())

			//			if err := vsphereinstall.AddToScheme(mgr.GetScheme()); err != nil {
			//				return errors.Wrap(err, "Could not update manager scheme")
			//			}

			if err := mgr.GetFieldIndexer().IndexField(ctx, &gardencorev1beta1.SecretBinding{}, index.SecretRefNamespaceField, index.SecretRefNamespaceIndexerFunc); err != nil {
				return err
			}

			if err := mgr.GetFieldIndexer().IndexField(ctx, &gardencorev1beta1.Shoot{}, index.SecretBindingNameField, index.SecretBindingNameIndexerFunc); err != nil {
				return err
			}

			log.Info("Setting up webhook server")
			//hookServer := mgr.GetWebhookServer()

			log.Info("Registering webhooks")
			//hookServer.Register("/webhooks/validate", &webhook.Admission{Handler: &validator.Shoot{Logger: log.WithName("shoot-validator")}})
			//hookServer.Register("/webhooks/mutate", &webhook.Admission{Handler: &mutator.Shoot{Logger: log.WithName("shoot-validator")}})

			if err := mgr.Start(ctx); err != nil {
				return errors.Wrap(err, "Error running manager")
			}

			return nil
		},
	}

	flags := cmd.Flags()
	aggOption.AddFlags(flags)
	verflag.AddFlags(flags)

	return cmd
}
