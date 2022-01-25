// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package mutator_test

import (
	"context"

	admissionmutator "github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/mutator"
	serviceinstall "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/install"
	servicev1alpha1 "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/v1alpha1"
	service2 "github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/pkg/apis/core/install"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	//"github.com/gardener/gardener/pkg/utils/test/matchers"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	//. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
)

type getDecoder interface {
	GetDecoder() runtime.Decoder
}

type partialShoot struct {
	providers []gardencorev1beta1.DNSProvider
	resources []gardencorev1beta1.NamedResourceReference
}

type dnsStyle int

const (
	dnsStyleNone     dnsStyle = 0
	dnsStyleDisabled dnsStyle = 1
	dnsStyleEnabled  dnsStyle = 2
)

var _ = Describe("Shoot Mutator", func() {
	var (
		scheme  *runtime.Scheme
		mutator extensionswebhook.Mutator
		domain  = "foo.domain.com"
		shoot   = &gardencorev1beta1.Shoot{
			Spec: gardencorev1beta1.ShootSpec{
				DNS: &gardencorev1beta1.DNS{
					Domain: &domain,
				},
				Extensions: []gardencorev1beta1.Extension{
					{Type: "shoot-cert-service"},
				},
			},
		}
		btrue              = true
		bfalse             = false
		secretName1        = "my-secret1"
		secretMappedName1  = "shoot-dns-service-my-secret1"
		secretName2        = "my-secret2"
		secretMappedName2  = "shoot-dns-service-my-secret2"
		shootWithResources = &gardencorev1beta1.Shoot{
			Spec: gardencorev1beta1.ShootSpec{
				DNS: &gardencorev1beta1.DNS{
					Domain: &domain,
				},
				Extensions: []gardencorev1beta1.Extension{
					{Type: "shoot-cert-service"},
					{
						Type: "shoot-dns-service",
						ProviderConfig: &runtime.RawExtension{
							Raw: []byte(`{"syncProvidersFromShootSpecDNS": true}`),
						},
						Disabled: &bfalse,
					},
				},
				Resources: []gardencorev1beta1.NamedResourceReference{
					{
						Name: secretMappedName2,
						ResourceRef: v1.CrossVersionObjectReference{
							Kind:       "Secret",
							Name:       "foo",
							APIVersion: "v1",
						},
					},
				},
			},
		}
		shootWithDisabledSync = &gardencorev1beta1.Shoot{
			Spec: gardencorev1beta1.ShootSpec{
				DNS: &gardencorev1beta1.DNS{
					Domain: &domain,
				},
				Extensions: []gardencorev1beta1.Extension{
					{
						Type: "shoot-dns-service",
						ProviderConfig: &runtime.RawExtension{
							Raw: []byte(`{"syncProvidersFromShootSpecDNS": false}`),
						},
						Disabled: &bfalse,
					},
				},
			},
		}
		dnsConfig = &servicev1alpha1.DNSConfig{
			TypeMeta: metav1.TypeMeta{
				APIVersion: servicev1alpha1.SchemeGroupVersion.String(),
				Kind:       "DNSConfig",
			},
			SyncProvidersFromShootSpecDNS: &btrue,
		}
		awsType = "aws-route53"
		primary = gardencorev1beta1.DNSProvider{
			Domains:    &gardencorev1beta1.DNSIncludeExclude{Include: []string{"my.domain.test"}, Exclude: []string{"private.my.domain.test"}},
			Type:       &awsType,
			SecretName: &secretName1,
			Primary:    &btrue,
		}
		primaryResource = gardencorev1beta1.NamedResourceReference{
			Name: secretMappedName1,
			ResourceRef: v1.CrossVersionObjectReference{
				Kind:       "Secret",
				Name:       secretName1,
				APIVersion: "v1",
			},
		}
		additional = gardencorev1beta1.DNSProvider{
			Zones:      &gardencorev1beta1.DNSIncludeExclude{Include: []string{"Z1234"}},
			Type:       &awsType,
			SecretName: &secretName2,
		}
		additionalResource = gardencorev1beta1.NamedResourceReference{
			Name: secretMappedName2,
			ResourceRef: v1.CrossVersionObjectReference{
				Kind:       "Secret",
				Name:       secretName2,
				APIVersion: "v1",
			},
		}
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		install.Install(scheme)
		serviceinstall.Install(scheme)
		mutator = admissionmutator.NewShootMutator()
		mutator.(inject.Scheme).InjectScheme(scheme)
	})

	DescribeTable("#Mutate",
		func(style dnsStyle, shootTemplate *gardencorev1beta1.Shoot, providers []gardencorev1beta1.DNSProvider, match gomegatypes.GomegaMatcher, expected *servicev1alpha1.DNSConfig, expectedResources []gardencorev1beta1.NamedResourceReference) {
			ctx := context.Background()
			oldShoot := shootTemplate.DeepCopy()
			newShoot := shootTemplate.DeepCopy()
			switch style {
			case dnsStyleNone:
				oldShoot.Spec.DNS = nil
				newShoot.Spec.DNS = nil
			case dnsStyleDisabled:
				newShoot.Spec.Extensions = append(newShoot.Spec.Extensions, gardencorev1beta1.Extension{
					Type:     service2.ExtensionType,
					Disabled: &btrue,
				})
			case dnsStyleEnabled:
				newShoot.Spec.DNS.Providers = providers
			}
			err := mutator.Mutate(ctx, newShoot, oldShoot)
			Expect(err).To(match)
			actual := findExtensionProviderConfig(mutator.(getDecoder).GetDecoder(), newShoot)
			if expected == nil {
				Expect(actual).To(BeNil())
			} else {
				Expect(actual).To(BeEquivalentTo(expected))
			}
			if expectedResources == nil {
				Expect(newShoot.Spec.Resources).To(BeNil())
			} else {
				Expect(newShoot.Spec.Resources).To(BeEquivalentTo(expectedResources))
			}
		},

		Entry("no DNS", dnsStyleNone, shoot, nil, BeNil(), nil, nil),
		Entry("extension disabled", dnsStyleDisabled, shoot, nil, BeNil(), nil, nil),
		Entry("extension enabled - default domain", dnsStyleEnabled, shoot, nil, BeNil(), modifyCopy(dnsConfig, func(cfg *servicev1alpha1.DNSConfig) {
			cfg.SyncProvidersFromShootSpecDNS = &btrue
		}), nil),
		Entry("primary", dnsStyleEnabled, shoot, []gardencorev1beta1.DNSProvider{primary}, BeNil(), modifyCopy(dnsConfig, func(cfg *servicev1alpha1.DNSConfig) {
			cfg.SyncProvidersFromShootSpecDNS = &btrue
			cfg.Providers = []servicev1alpha1.DNSProvider{
				{
					Domains: &servicev1alpha1.DNSIncludeExclude{
						Include: []string{"my.domain.test"},
						Exclude: []string{"private.my.domain.test"},
					},
					Primary:    &btrue,
					SecretName: &secretMappedName1,
					Type:       &awsType,
				},
			}
		}), []gardencorev1beta1.NamedResourceReference{primaryResource}),
		Entry("primary+additional", dnsStyleEnabled, shootWithResources, []gardencorev1beta1.DNSProvider{primary, additional}, BeNil(), modifyCopy(dnsConfig, func(cfg *servicev1alpha1.DNSConfig) {
			cfg.SyncProvidersFromShootSpecDNS = &btrue
			cfg.Providers = []servicev1alpha1.DNSProvider{
				{
					Domains: &servicev1alpha1.DNSIncludeExclude{
						Include: []string{"my.domain.test"},
						Exclude: []string{"private.my.domain.test"},
					},
					Primary:    &btrue,
					SecretName: &secretMappedName1,
					Type:       &awsType,
				},
				{
					SecretName: &secretMappedName2,
					Type:       &awsType,
					Zones: &servicev1alpha1.DNSIncludeExclude{
						Include: []string{"Z1234"},
					},
				},
			}
		}), []gardencorev1beta1.NamedResourceReference{additionalResource, primaryResource}),
		Entry("disabled sync", dnsStyleEnabled, shootWithDisabledSync, []gardencorev1beta1.DNSProvider{additional}, BeNil(), modifyCopy(dnsConfig, func(cfg *servicev1alpha1.DNSConfig) {
			cfg.SyncProvidersFromShootSpecDNS = &bfalse
		}), nil),
	)
})

func findExtensionProviderConfig(decoder runtime.Decoder, shoot *gardencorev1beta1.Shoot) *servicev1alpha1.DNSConfig {
	for _, ext := range shoot.Spec.Extensions {
		if ext.Type == service2.ExtensionType && ext.ProviderConfig != nil && ext.ProviderConfig.Raw != nil {
			dnsConfig := &servicev1alpha1.DNSConfig{}
			_, _, err := decoder.Decode(ext.ProviderConfig.Raw, nil, dnsConfig)
			Expect(err).To(BeNil())
			return dnsConfig
		}
	}
	return nil
}

func modifyCopy(original *servicev1alpha1.DNSConfig, modifier func(*servicev1alpha1.DNSConfig)) *servicev1alpha1.DNSConfig {
	new := original.DeepCopy()
	modifier(new)
	return new
}
