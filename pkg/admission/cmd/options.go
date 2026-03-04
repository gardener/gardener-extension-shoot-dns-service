// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package cmd

import (
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	webhookcmd "github.com/gardener/gardener/extensions/pkg/webhook/cmd"
	"github.com/spf13/pflag"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/mutator"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/validator"
)

// GardenWebhookSwitchOptions are the webhookcmd.SwitchOptions for the admission webhooks.
func GardenWebhookSwitchOptions() *webhookcmd.SwitchOptions {
	pairs := []webhookcmd.NameToFactory{
		webhookcmd.Switch(validator.ValidatorName, validator.New),
		webhookcmd.Switch(mutator.MutatorName, mutator.New),
	}
	pairs = append(pairs, validator.NewWorkloadIdentityWebhooks()...)
	return webhookcmd.NewSwitchOptions(pairs...)

}

// ConfigOptions are command line options that can be set for admission webhooks.
type ConfigOptions struct {
	GCPWorkloadIdentityOptions GCPWorkloadIdentityOptions

	config *config.InternalGCPWorkloadIdentityConfig
}

// GCPWorkloadIdentityOptions are options that specify how GCP workload identities should be validated.
type GCPWorkloadIdentityOptions struct {
	// AllowedTokenURLs are the allowed token URLs.
	AllowedTokenURLs []string
	// AllowedServiceAccountImpersonationURLRegExps are the allowed service account impersonation URL regular expressions.
	AllowedServiceAccountImpersonationURLRegExps []string
}

// AddFlags implements Flagger.AddFlags.
func (w *GCPWorkloadIdentityOptions) AddFlags(fs *pflag.FlagSet) {
	fs.StringSliceVar(
		&w.AllowedTokenURLs,
		"wi-gcp-allowed-token-url",
		[]string{"https://sts.googleapis.com/v1/token"},
		"Token URL that is allowed to be configured in GCP workload identity configurations. Can be set multiple times.",
	)
	fs.StringSliceVar(
		&w.AllowedServiceAccountImpersonationURLRegExps,
		"wi-gcp-allowed-service-account-impersonation-url-regexp",
		[]string{`^https://iamcredentials\.googleapis\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$`},
		"Regular expression that is used to validate service account impersonation urls configured in GCP workload identity configurations. Can be set multiple times.",
	)
}

// Complete implements RESTCompleter.Complete.
func (c *ConfigOptions) Complete() error {
	var err error
	c.config, err = config.NewInternalGCPWorkloadIdentityConfig(config.GCPWorkloadIdentityConfig{
		AllowedTokenURLs: c.GCPWorkloadIdentityOptions.AllowedTokenURLs,
		AllowedServiceAccountImpersonationURLRegExps: c.GCPWorkloadIdentityOptions.AllowedServiceAccountImpersonationURLRegExps,
	})
	return err
}

// Completed returns the completed Config. Only call this if `Complete` was successful.
func (c *ConfigOptions) Completed() *config.InternalGCPWorkloadIdentityConfig {
	return c.config
}

// AddFlags implements Flagger.AddFlags.
func (c *ConfigOptions) AddFlags(fs *pflag.FlagSet) {
	c.GCPWorkloadIdentityOptions.AddFlags(fs)
}
