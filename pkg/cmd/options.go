// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"fmt"
	"strings"
	"time"

	dnsman2apisconfig "github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	healthcheckconfigv1alpha1 "github.com/gardener/gardener/extensions/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener/extensions/pkg/controller/cmd"
	extensionshealthcheckcontroller "github.com/gardener/gardener/extensions/pkg/controller/healthcheck"
	extensionsheartbeatcontroller "github.com/gardener/gardener/extensions/pkg/controller/heartbeat"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	admissioncmd "github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/cmd"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/healthcheck"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/lifecycle"
)

// DNSServiceOptions holds options related to the dns service.
type DNSServiceOptions struct {
	SeedID                     string
	DNSClass                   string
	ManageDNSProviders         bool
	ReplicateDNSProviders      bool
	RemoteDefaultDomainSecret  string
	GCPWorkloadIdentityOptions admissioncmd.GCPWorkloadIdentityOptions
	config                     *DNSServiceConfig
}

// HealthOptions holds options for health checks.
type HealthOptions struct {
	HealthCheckSyncPeriod time.Duration
	config                *HealthConfig
}

// AddFlags implements Flagger.AddFlags.
func (o *DNSServiceOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.SeedID, "seed-id", "", "ID of the current cluster")
	fs.StringVar(&o.DNSClass, "dns-class", "garden", "DNS class used to filter DNS source resources in shoot clusters")
	fs.BoolVar(&o.ManageDNSProviders, "manage-dns-providers", false, "enables management of DNSProviders in control plane (must only be enable if Gardenlet has disabled it)")
	fs.BoolVar(&o.ReplicateDNSProviders, "replicate-dns-providers", false, "enables replication of DNSProviders from shoot cluster to seed cluster")
	fs.StringVar(&o.RemoteDefaultDomainSecret, "remote-default-domain-secret", "", "secret name for default 'external' DNSProvider DNS class used to filter DNS source resources in shoot clusters")
	o.GCPWorkloadIdentityOptions.AddFlags(fs)
}

// AddFlags implements Flagger.AddFlags.
func (o *HealthOptions) AddFlags(fs *pflag.FlagSet) {
	fs.DurationVar(&o.HealthCheckSyncPeriod, "healthcheck-sync-period", time.Second*30, "sync period for the health check controller")
}

// Complete implements Completer.Complete.
func (o *DNSServiceOptions) Complete() error {
	var remoteDefaultDomainSecret *types.NamespacedName
	if o.RemoteDefaultDomainSecret != "" {
		parts := strings.Split(o.RemoteDefaultDomainSecret, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid format for remote-default-domain-secret: %s (expected '<namespace>/<name>')", o.RemoteDefaultDomainSecret)
		}
		remoteDefaultDomainSecret = &types.NamespacedName{
			Namespace: parts[0],
			Name:      parts[1],
		}
	}

	gcpGCPWorkloadIdentityConfig, err := dnsman2apisconfig.NewInternalGCPWorkloadIdentityConfig(dnsman2apisconfig.GCPWorkloadIdentityConfig{
		AllowedTokenURLs: o.GCPWorkloadIdentityOptions.AllowedTokenURLs,
		AllowedServiceAccountImpersonationURLRegExps: o.GCPWorkloadIdentityOptions.AllowedServiceAccountImpersonationURLRegExps,
	})
	if err != nil {
		return err
	}

	o.config = &DNSServiceConfig{
		SeedID:                            o.SeedID,
		DNSClass:                          o.DNSClass,
		ManageDNSProviders:                o.ManageDNSProviders,
		ReplicateDNSProviders:             o.ReplicateDNSProviders,
		RemoteDefaultDomainSecret:         remoteDefaultDomainSecret,
		InternalGCPWorkloadIdentityConfig: *gcpGCPWorkloadIdentityConfig,
	}
	return nil
}

// Complete implements Completer.Complete.
func (o *HealthOptions) Complete() error {
	o.config = &HealthConfig{HealthCheckSyncPeriod: metav1.Duration{Duration: o.HealthCheckSyncPeriod}}
	return nil
}

// Completed returns the decoded CertificatesServiceConfiguration instance. Only call this if `Complete` was successful.
func (o *DNSServiceOptions) Completed() *DNSServiceConfig {
	return o.config
}

// Completed returns the completed HealthOptions. Only call this if `Complete` was successful.
func (o *HealthOptions) Completed() *HealthConfig {
	return o.config
}

// DNSServiceConfig contains configuration information about the dns service.
type DNSServiceConfig struct {
	SeedID                            string
	DNSClass                          string
	ManageDNSProviders                bool
	ReplicateDNSProviders             bool
	RemoteDefaultDomainSecret         *types.NamespacedName
	InternalGCPWorkloadIdentityConfig dnsman2apisconfig.InternalGCPWorkloadIdentityConfig
}

// Apply applies the DNSServiceOptions to the passed ControllerOptions instance.
func (c *DNSServiceConfig) Apply(cfg *config.DNSServiceConfig) {
	cfg.SeedID = c.SeedID
	cfg.DNSClass = c.DNSClass
	cfg.ReplicateDNSProviders = c.ReplicateDNSProviders
	cfg.ManageDNSProviders = c.ManageDNSProviders
	cfg.RemoteDefaultDomainSecret = c.RemoteDefaultDomainSecret
	cfg.InternalGCPWorkloadIdentityConfig = c.InternalGCPWorkloadIdentityConfig
}

// HealthConfig contains configuration information about the health check controller.
type HealthConfig struct {
	HealthCheckSyncPeriod metav1.Duration
}

// ApplyHealthCheckConfig applies the `HealthConfig` to the passed health configurtaion.
func (c *HealthConfig) ApplyHealthCheckConfig(config *healthcheckconfigv1alpha1.HealthCheckConfig) {
	config.SyncPeriod = c.HealthCheckSyncPeriod
}

// ControllerSwitches are the cmd.ControllerSwitches for the provider controllers.
func ControllerSwitches() *cmd.SwitchOptions {
	return cmd.NewSwitchOptions(
		cmd.Switch(lifecycle.Name, lifecycle.AddToManager),
		cmd.Switch(extensionshealthcheckcontroller.ControllerName, healthcheck.RegisterHealthChecks),
		cmd.Switch(extensionsheartbeatcontroller.ControllerName, extensionsheartbeatcontroller.AddToManager),
	)
}
