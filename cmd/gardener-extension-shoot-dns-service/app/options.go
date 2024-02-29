// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"os"

	controllercmd "github.com/gardener/gardener/extensions/pkg/controller/cmd"
	heartbeatcmd "github.com/gardener/gardener/extensions/pkg/controller/heartbeat/cmd"

	dnsservicecmd "github.com/gardener/gardener-extension-shoot-dns-service/pkg/cmd"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

// ExtensionName is the name of the extension.
const ExtensionName = service.ExtensionServiceName

// Options holds configuration passed to the DNS Service controller.
type Options struct {
	generalOptions               *controllercmd.GeneralOptions
	serviceOptions               *dnsservicecmd.DNSServiceOptions
	healthOptions                *dnsservicecmd.HealthOptions
	restOptions                  *controllercmd.RESTOptions
	managerOptions               *controllercmd.ManagerOptions
	lifecycleControllerOptions   *controllercmd.ControllerOptions
	healthControllerOptions      *controllercmd.ControllerOptions
	heartbeatControllerOptions   *heartbeatcmd.Options
	replicationControllerOptions *controllercmd.ControllerOptions
	controllerSwitches           *controllercmd.SwitchOptions
	reconcileOptions             *controllercmd.ReconcilerOptions
	optionAggregator             controllercmd.OptionAggregator
}

// NewOptions creates a new Options instance.
func NewOptions() *Options {
	options := &Options{
		generalOptions: &controllercmd.GeneralOptions{},
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
		heartbeatControllerOptions: &heartbeatcmd.Options{
			// This is a default value.
			ExtensionName:        ExtensionName,
			RenewIntervalSeconds: 30,
			Namespace:            os.Getenv("LEADER_ELECTION_NAMESPACE"),
		},
		replicationControllerOptions: &controllercmd.ControllerOptions{
			// This is a default value.
			MaxConcurrentReconciles: 5,
		},
		controllerSwitches: dnsservicecmd.ControllerSwitches(),
		reconcileOptions:   &controllercmd.ReconcilerOptions{},
	}

	options.optionAggregator = controllercmd.NewOptionAggregator(
		options.generalOptions,
		options.serviceOptions,
		options.healthOptions,
		options.restOptions,
		options.managerOptions,
		controllercmd.PrefixOption("lifecycle-", options.lifecycleControllerOptions),
		controllercmd.PrefixOption("healthcheck-", options.healthControllerOptions),
		controllercmd.PrefixOption("replication-", options.replicationControllerOptions),
		controllercmd.PrefixOption("heartbeat-", options.heartbeatControllerOptions),
		options.controllerSwitches,
		options.reconcileOptions,
	)

	return options
}
