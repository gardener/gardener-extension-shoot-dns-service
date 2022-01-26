// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

	serviceinstall "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/install"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/healthcheck"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/lifecycle"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/replication"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"

	dnsapi "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/util"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	componentbaseconfig "k8s.io/component-base/config"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// NewServiceControllerCommand creates a new command that is used to start the DNS Service controller.
func NewServiceControllerCommand() *cobra.Command {
	options := NewOptions()

	cmd := &cobra.Command{
		Use:           service.ServiceName + "-extension-controller-manager",
		Short:         "DNS Meta Controller for Shoots.",
		SilenceErrors: true,

		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.optionAggregator.Complete(); err != nil {
				return fmt.Errorf("error completing options: %s", err)
			}
			cmd.SilenceUsage = true
			return options.run(cmd.Context())
		},
	}

	options.optionAggregator.AddFlags(cmd.Flags())

	return cmd
}

func (o *Options) run(ctx context.Context) error {
	// TODO: Make these flags configurable via command line parameters or component config file.
	util.ApplyClientConnectionConfigurationToRESTConfig(&componentbaseconfig.ClientConnectionConfiguration{
		QPS:   100.0,
		Burst: 130,
	}, o.restOptions.Completed().Config)

	mgrScheme := runtime.NewScheme()
	if err := scheme.AddToScheme(mgrScheme); err != nil {
		return fmt.Errorf("could not update manager scheme (kubernetes): %s", err)
	}
	if err := dnsapi.AddToScheme(mgrScheme); err != nil {
		return fmt.Errorf("could not update manager scheme (dnsapi): %s", err)
	}
	if err := serviceinstall.AddToScheme(mgrScheme); err != nil {
		return fmt.Errorf("could not update manager scheme: %s", err)
	}
	if err := extensionscontroller.AddToScheme(mgrScheme); err != nil {
		return fmt.Errorf("could not update manager scheme: %s", err)
	}

	useTokenRequestor, err := extensionscontroller.UseTokenRequestor(o.generalOptions.Completed().GardenerVersion)
	if err != nil {
		return fmt.Errorf("could not determine whether token requestor should be used: %s", err)
	}
	lifecycle.DefaultAddOptions.UseTokenRequestor = useTokenRequestor

	useProjectedTokenMount, err := extensionscontroller.UseServiceAccountTokenVolumeProjection(o.generalOptions.Completed().GardenerVersion)
	if err != nil {
		return fmt.Errorf("could not determine whether service account token volume projection should be used: %s", err)
	}
	lifecycle.DefaultAddOptions.UseProjectedTokenMount = useProjectedTokenMount

	mgrOpts := o.managerOptions.Completed().Options()
	mgrOpts.Scheme = mgrScheme
	mgrOpts.ClientDisableCacheFor = []client.Object{
		&corev1.Secret{},    // applied for ManagedResources
		&corev1.ConfigMap{}, // applied for monitoring config
		&dnsapi.DNSOwner{},  // avoid watching DNSOwner
	}
	mgr, err := manager.New(o.restOptions.Completed().Config, mgrOpts)
	if err != nil {
		return fmt.Errorf("could not instantiate controller-manager: %s", err)
	}

	o.serviceOptions.Completed().Apply(&config.DNSService)
	o.healthOptions.Completed().ApplyHealthCheckConfig(&healthcheck.DefaultAddOptions.HealthCheckConfig)
	o.healthControllerOptions.Completed().Apply(&healthcheck.DefaultAddOptions.Controller)
	o.lifecycleControllerOptions.Completed().Apply(&lifecycle.DefaultAddOptions.Controller)
	o.replicationControllerOptions.Completed().Apply(&replication.DefaultAddOptions.Controller)
	o.reconcileOptions.Completed().Apply(&lifecycle.DefaultAddOptions.IgnoreOperationAnnotation)

	if err := o.controllerSwitches.Completed().AddToManager(mgr); err != nil {
		return fmt.Errorf("could not add controllers to manager: %s", err)
	}

	if err := mgr.Start(ctx); err != nil {
		return fmt.Errorf("error running manager: %s", err)
	}

	return nil
}
