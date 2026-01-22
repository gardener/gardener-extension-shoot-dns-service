// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"context"
	"fmt"
	"reflect"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/pkg/apis/core"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
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
		client:  mgr.GetClient(),
	}
}

// shoot validates shoots
type shoot struct {
	decoder runtime.Decoder
	client  client.Client
}

// Validate implements extensionswebhook.Validator.Validate
func (s *shoot) Validate(ctx context.Context, new, old client.Object) error {
	shoot, ok := new.(*core.Shoot)
	if !ok {
		return fmt.Errorf("wrong object type %T", new)
	}
	var oldShoot *core.Shoot
	if old != nil {
		oldShoot, ok = old.(*core.Shoot)
		if !ok {
			return fmt.Errorf("wrong object type %T (old)", old)
		}
	}

	return s.validateShoot(ctx, shoot, oldShoot)
}

func (s *shoot) validateShoot(ctx context.Context, shoot, oldShoot *core.Shoot) error {
	if s.isDisabled(shoot) {
		return nil
	}
	dnsConfig, err := s.extractDNSConfig(shoot)
	if err != nil {
		return err
	}
	var oldDnsConfig *apisservice.DNSConfig
	if oldShoot != nil {
		oldDnsConfig, err = s.extractDNSConfig(oldShoot)
		if err != nil {
			// If we cannot extract the old DNSConfig, we handle it as if it's a create operation
			oldDnsConfig = nil
		}
	}

	allErrs := field.ErrorList{}
	if dnsConfig != nil {
		var getter validation.ResourceGetter
		if hasChanged := oldDnsConfig == nil || !reflect.DeepEqual(dnsConfig, oldDnsConfig); hasChanged {
			// If the DNSConfig has changed, we want to validate the secrets.
			// Otherwise, we skip the secret validation to avoid shoot manifests updates to fail due to an unrelated changed secret.
			getter = s.makeSecretGetter(ctx, shoot.Namespace)
		}
		allErrs = append(allErrs, validation.ValidateDNSConfig(dnsConfig, &shoot.Spec.Resources, getter)...)
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

type resourceGetter struct {
	ctx                               context.Context
	client                            client.Client
	namespace                         string
	internalGCPWorkloadIdentityConfig config.InternalGCPWorkloadIdentityConfig
}

func (r *resourceGetter) GetSecret(name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	err := r.client.Get(r.ctx, client.ObjectKey{Namespace: r.namespace, Name: name}, secret)
	return secret, err
}

func (r *resourceGetter) GetWorkloadIdentity(name string) (*securityv1alpha1.WorkloadIdentity, error) {
	wl := &securityv1alpha1.WorkloadIdentity{}
	err := r.client.Get(r.ctx, client.ObjectKey{Namespace: r.namespace, Name: name}, wl)
	return wl, err
}

func (r *resourceGetter) GetInternalGCPWorkloadIdentityConfig() config.InternalGCPWorkloadIdentityConfig {
	return r.internalGCPWorkloadIdentityConfig
}

func (s *shoot) makeSecretGetter(ctx context.Context, namespace string) validation.ResourceGetter {
	return &resourceGetter{
		ctx:       ctx,
		client:    s.client,
		namespace: namespace,
	}
}
