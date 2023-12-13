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

package validator

import (
	"context"
	"fmt"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/pkg/apis/core"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	apisservice "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/validation"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

// NewShootValidator returns a new instance of a shoot validator.
func NewShootValidator(mgr manager.Manager) extensionswebhook.Validator {
	return &shoot{
		decoder: serializer.NewCodecFactory(mgr.GetScheme()).UniversalDecoder(),
	}
}

// shoot validates shoots
type shoot struct {
	decoder runtime.Decoder
}

// Validate implements extensionswebhook.Validator.Validate
func (s *shoot) Validate(ctx context.Context, new, _ client.Object) error {
	shoot, ok := new.(*core.Shoot)
	if !ok {
		return fmt.Errorf("wrong object type %T", new)
	}

	return s.validateShoot(ctx, shoot)
}

func (s *shoot) validateShoot(_ context.Context, shoot *core.Shoot) error {
	if s.isDisabled(shoot) {
		return nil
	}
	dnsConfig, err := s.extractDNSConfig(shoot)
	if err != nil {
		return err
	}

	allErrs := field.ErrorList{}
	if dnsConfig != nil {
		allErrs = append(allErrs, validation.ValidateDNSConfig(dnsConfig, shoot.Spec.Resources)...)
	}

	return allErrs.ToAggregate()
}

// isDisabled returns true if extension is explicitly disabled.
func (s *shoot) isDisabled(shoot *core.Shoot) bool {
	ext := s.findExtension(shoot)
	if ext == nil {
		return false
	}
	if ext.Disabled != nil {
		return *ext.Disabled
	}
	return false
}

// extractDNSConfig extracts DNSConfig from providerConfig.
func (s *shoot) extractDNSConfig(shoot *core.Shoot) (*apisservice.DNSConfig, error) {
	ext := s.findExtension(shoot)
	if ext != nil && ext.ProviderConfig != nil {
		dnsConfig := &apisservice.DNSConfig{}
		if _, _, err := s.decoder.Decode(ext.ProviderConfig.Raw, nil, dnsConfig); err != nil {
			return nil, fmt.Errorf("failed to decode %s provider config: %w", ext.Type, err)
		}
		return dnsConfig, nil
	}

	return nil, nil
}

// findExtension returns shoot-dns-service extension.
func (s *shoot) findExtension(shoot *core.Shoot) *core.Extension {
	for i, ext := range shoot.Spec.Extensions {
		if ext.Type == service.ExtensionType {
			return &shoot.Spec.Extensions[i]
		}
	}
	return nil
}
