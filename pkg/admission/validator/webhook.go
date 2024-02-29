// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/pkg/apis/core"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// ValidatorName is a common name for a validation webhook.
	ValidatorName = "validator"
	// ValidatorPath is a common path for a validation webhook.
	ValidatorPath = "/webhooks/validate"
)

var logger = log.Log.WithName("shoot-dns-service-validator-webhook")

// New creates a new webhook that validates Shoot resources.
func New(mgr manager.Manager) (*extensionswebhook.Webhook, error) {
	logger.Info("Setting up webhook", "name", ValidatorName)

	return extensionswebhook.New(mgr, extensionswebhook.Args{
		Provider: "shoot-dns-service",
		Name:     ValidatorName,
		Path:     ValidatorPath,
		Validators: map[extensionswebhook.Validator][]extensionswebhook.Type{
			NewShootValidator(mgr): {{Obj: &core.Shoot{}}},
		},
		Target: extensionswebhook.TargetSeed,
		ObjectSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"extensions.extensions.gardener.cloud/shoot-dns-service": "true"},
		},
	})
}
