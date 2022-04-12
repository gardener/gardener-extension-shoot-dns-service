// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	"os"

	"github.com/spf13/cobra"
	componentbaseconfig "k8s.io/component-base/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	runtimelog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	admissioncmd "github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/cmd"
	serviceinstall "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/install"
	controllercmd "github.com/gardener/gardener/extensions/pkg/controller/cmd"
	"github.com/gardener/gardener/extensions/pkg/util"
	webhookcmd "github.com/gardener/gardener/extensions/pkg/webhook/cmd"
	"github.com/gardener/gardener/pkg/apis/core/install"
)

var log = logf.Log.WithName("gardener-extension-admission-shoot-dns-service")

// NewAdmissionCommand creates a new command for running an AWS admission webhook.
func NewAdmissionCommand(ctx context.Context) *cobra.Command {
	var (
		restOpts = &controllercmd.RESTOptions{}
		mgrOpts  = &controllercmd.ManagerOptions{
			WebhookServerPort: 443,
		}
		webhookSwitches = admissioncmd.GardenWebhookSwitchOptions()
		webhookOptions  = webhookcmd.NewAddToManagerSimpleOptions(webhookSwitches)

		aggOption = controllercmd.NewOptionAggregator(
			restOpts,
			mgrOpts,
			webhookOptions,
		)
	)

	cmd := &cobra.Command{
		Use: "admission webhooks of shoot-dns-service",

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := aggOption.Complete(); err != nil {
				runtimelog.Log.Error(err, "Error completing options")
				os.Exit(1)
			}

			util.ApplyClientConnectionConfigurationToRESTConfig(&componentbaseconfig.ClientConnectionConfiguration{
				QPS:   100.0,
				Burst: 130,
			}, restOpts.Completed().Config)

			mgr, err := manager.New(restOpts.Completed().Config, mgrOpts.Completed().Options())
			if err != nil {
				runtimelog.Log.Error(err, "Could not instantiate manager")
				os.Exit(1)
			}

			install.Install(mgr.GetScheme())

			if err := serviceinstall.AddToScheme(mgr.GetScheme()); err != nil {
				runtimelog.Log.Error(err, "Could not update manager scheme")
				os.Exit(1)
			}

			log.Info("Setting up webhook server")
			if err := webhookOptions.Completed().AddToManager(mgr); err != nil {
				return err
			}

			return mgr.Start(ctx)
		},
	}

	aggOption.AddFlags(cmd.Flags())

	return cmd
}
