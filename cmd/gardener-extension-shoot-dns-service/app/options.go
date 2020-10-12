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
	"os"

	dnsservicecmd "github.com/gardener/gardener-extension-shoot-dns-service/pkg/cmd"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"

	controllercmd "github.com/gardener/gardener/extensions/pkg/controller/cmd"
)

// ExtensionName is the name of the extension.
const ExtensionName = service.ExtensionServiceName

// Options holds configuration passed to the DNS Service controller.
type Options struct {
	serviceOptions               *dnsservicecmd.DNSServiceOptions
	healthOptions                *dnsservicecmd.HealthOptions
	restOptions                  *controllercmd.RESTOptions
	managerOptions               *controllercmd.ManagerOptions
	lifecycleControllerOptions   *controllercmd.ControllerOptions
	healthControllerOptions      *controllercmd.ControllerOptions
	replicationControllerOptions *controllercmd.ControllerOptions
	controllerSwitches           *controllercmd.SwitchOptions
	reconcileOptions             *controllercmd.ReconcilerOptions
	optionAggregator             controllercmd.OptionAggregator
}

// NewOptions creates a new Options instance.
func NewOptions() *Options {
	options := &Options{
		serviceOptions: &dnsservicecmd.DNSServiceOptions{},
		healthOptions:  &dnsservicecmd.HealthOptions{},
		restOptions:    &controllercmd.RESTOptions{},
		managerOptions: &controllercmd.ManagerOptions{
			// These are default values.
			LeaderElection:          true,
			LeaderElectionID:        controllercmd.LeaderElectionNameID(ExtensionName),
			LeaderElectionNamespace: os.Getenv("LEADER_ELECTION_NAMESPACE"),
		},
		lifecycleControllerOptions: &controllercmd.ControllerOptions{
			// This is a default value.
			MaxConcurrentReconciles: 5,
		},
		healthControllerOptions: &controllercmd.ControllerOptions{
			// This is a default value.
			MaxConcurrentReconciles: 5,
		},
		replicationControllerOptions: &controllercmd.ControllerOptions{
			// This is a default value.
			MaxConcurrentReconciles: 5,
		},
		controllerSwitches: dnsservicecmd.ControllerSwitches(),
		reconcileOptions:   &controllercmd.ReconcilerOptions{},
	}

	options.optionAggregator = controllercmd.NewOptionAggregator(
		options.serviceOptions,
		options.healthOptions,
		options.restOptions,
		options.managerOptions,
		controllercmd.PrefixOption("lifecycle-", options.lifecycleControllerOptions),
		controllercmd.PrefixOption("healthcheck-", options.healthControllerOptions),
		controllercmd.PrefixOption("replication-", options.replicationControllerOptions),
		options.controllerSwitches,
		options.reconcileOptions,
	)

	return options
}
