// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"fmt"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	webhookcmd "github.com/gardener/gardener/extensions/pkg/webhook/cmd"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// WorkloadIdentityValidatorNameFormat is a common name for a validation webhook.
	WorkloadIdentityValidatorNameFormat = "validator-workload-identity-%s"
	// WorkloadIdentityValidatorPathFormat is a common path for a validation webhook.
	WorkloadIdentityValidatorPathFormat = "/webhooks/validate-workload-identity-%s"
)

var (
	// DefaultAddOptions are the default AddOptions for configuring the validator.
	DefaultAddOptions = AddOptions{}
)

// AddOptions are options to apply when adding the GCP admission webhook to the manager.
type AddOptions struct {
	// GCPWorkloadIdentityConfig is the GCP workload identity validation configuration.
	GCPWorkloadIdentityConfig config.InternalGCPWorkloadIdentityConfig
}

// NewWorkloadIdentityWebhooks creates a new webhooks that validates provider dependent WorkloadIdentity resources.
func NewWorkloadIdentityWebhooks() []webhookcmd.NameToFactory {
	// Separate webhooks for each provider type are needed, because each
	// provider type has its own label key.
	// Object selectors cannot be combined with multiple ORed labels in a single webhook.
	var pairs []webhookcmd.NameToFactory
	for _, providerType := range []string{"aws", "azure", "gcp"} {
		validatorName := fmt.Sprintf(WorkloadIdentityValidatorNameFormat, providerType)
		validatorPath := fmt.Sprintf(WorkloadIdentityValidatorPathFormat, providerType)
		pairs = append(pairs, webhookcmd.NameToFactory{
			Name: validatorName,
			Func: func(mgr manager.Manager) (*extensionswebhook.Webhook, error) {
				logger.Info("Setting up webhook", "name", validatorName, "providerType", providerType)
				return extensionswebhook.New(mgr, extensionswebhook.Args{
					Provider: "shoot-dns-service",
					Name:     validatorName,
					Path:     validatorPath,
					Validators: map[extensionswebhook.Validator][]extensionswebhook.Type{
						NewWorkloadIdentityValidator(DefaultAddOptions.GCPWorkloadIdentityConfig): {{Obj: &securityv1alpha1.WorkloadIdentity{}}},
					},
					Target: extensionswebhook.TargetSeed,
					ObjectSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{v1beta1constants.LabelExtensionProviderTypePrefix + providerType: "true"},
					},
				})
			},
		})
	}
	return pairs
}
