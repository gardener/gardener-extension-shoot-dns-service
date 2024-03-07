// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package mutator_test

import (
	"context"
	"time"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	"github.com/gardener/gardener/pkg/apis/core/install"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	mockmanager "github.com/gardener/gardener/third_party/mock/controller-runtime/manager"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	"go.uber.org/mock/gomock"
	v1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	admissionmutator "github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/mutator"
	serviceinstall "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/install"
	servicev1alpha1 "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/v1alpha1"
	service2 "github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

type dnsStyle int

const (
	dnsStyleNone     dnsStyle = 0
	dnsStyleDisabled dnsStyle = 1
	dnsStyleEnabled  dnsStyle = 2
)

var _ = Describe("Shoot Mutator", func() {
	var (
		scheme *runtime.Scheme
		ctrl   *gomock.Controller
		mgr    *mockmanager.MockManager

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
						Name: "shoot-dns-service-my-secret-obsolete1",
						ResourceRef: v1.CrossVersionObjectReference{
							Kind:       "Secret",
							Name:       "foo",
							APIVersion: "v1",
						},
					},
					{
						Name: secretMappedName2,
						ResourceRef: v1.CrossVersionObjectReference{
							Kind:       "Secret",
							Name:       "foo",
							APIVersion: "v1",
						},
					},
					{
						Name: "shoot-dns-service-my-secret-obsolete2",
						ResourceRef: v1.CrossVersionObjectReference{
							Kind:       "Secret",
							Name:       "foo",
							APIVersion: "v1",
						},
					},
					{
						Name: "other",
						ResourceRef: v1.CrossVersionObjectReference{
							Kind:       "Secret",
							Name:       "other",
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
		shootInDeletion = &gardencorev1beta1.Shoot{
			ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &metav1.Time{Time: time.Now()}},
			Spec: gardencorev1beta1.ShootSpec{
				DNS: &gardencorev1beta1.DNS{
					Domain: &domain,
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
		awsType        = "aws-route53"
		primaryDefault = gardencorev1beta1.DNSProvider{
			Type:       &awsType,
			SecretName: &secretName1,
			Primary:    &btrue,
		}
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
		otherResource = gardencorev1beta1.NamedResourceReference{
			Name: "other",
			ResourceRef: v1.CrossVersionObjectReference{
				Kind:       "Secret",
				Name:       "other",
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

		ctrl = gomock.NewController(GinkgoT())
		mgr = mockmanager.NewMockManager(ctrl)
		mgr.EXPECT().GetScheme().Return(scheme).Times(3)

		mutator = admissionmutator.NewShootMutator(mgr)
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
			actual := findExtensionProviderConfig(serializer.NewCodecFactory(mgr.GetScheme()).UniversalDecoder(), newShoot)
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
		Entry("primaryDefault", dnsStyleEnabled, shoot, []gardencorev1beta1.DNSProvider{primaryDefault}, BeNil(), modifyCopy(dnsConfig, func(cfg *servicev1alpha1.DNSConfig) {
			cfg.SyncProvidersFromShootSpecDNS = &btrue
			cfg.Providers = []servicev1alpha1.DNSProvider{
				{
					Domains: &servicev1alpha1.DNSIncludeExclude{
						Include: []string{domain},
					},
					SecretName: &secretMappedName1,
					Type:       &awsType,
				},
			}
		}), []gardencorev1beta1.NamedResourceReference{primaryResource}),
		Entry("primary", dnsStyleEnabled, shoot, []gardencorev1beta1.DNSProvider{primary}, BeNil(), modifyCopy(dnsConfig, func(cfg *servicev1alpha1.DNSConfig) {
			cfg.SyncProvidersFromShootSpecDNS = &btrue
			cfg.Providers = []servicev1alpha1.DNSProvider{
				{
					Domains: &servicev1alpha1.DNSIncludeExclude{
						Include: []string{"my.domain.test"},
						Exclude: []string{"private.my.domain.test"},
					},
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
		}), []gardencorev1beta1.NamedResourceReference{additionalResource, otherResource, primaryResource}),
		Entry("disabled sync", dnsStyleEnabled, shootWithDisabledSync, []gardencorev1beta1.DNSProvider{additional}, BeNil(), modifyCopy(dnsConfig, func(cfg *servicev1alpha1.DNSConfig) {
			cfg.SyncProvidersFromShootSpecDNS = &bfalse
		}), nil),
		Entry("shoot in deletion", dnsStyleEnabled, shootInDeletion, []gardencorev1beta1.DNSProvider{additional}, BeNil(), nil, nil),
	)
})

func findExtensionProviderConfig(decoder runtime.Decoder, shoot *gardencorev1beta1.Shoot) *servicev1alpha1.DNSConfig {
	for _, ext := range shoot.Spec.Extensions {
		if ext.Type == service2.ExtensionType && ext.ProviderConfig != nil && ext.ProviderConfig.Raw != nil {
			dnsConfig := &servicev1alpha1.DNSConfig{
				TypeMeta: metav1.TypeMeta{
					Kind:       "DNSConfig",
					APIVersion: "service.dns.extensions.gardener.cloud/v1alpha1",
				},
			}
			_, _, err := decoder.Decode(ext.ProviderConfig.Raw, nil, dnsConfig)
			Expect(err).To(BeNil())
			return dnsConfig
		}
	}
	return nil
}

func modifyCopy(original *servicev1alpha1.DNSConfig, modifier func(*servicev1alpha1.DNSConfig)) *servicev1alpha1.DNSConfig {
	cfg := original.DeepCopy()
	modifier(cfg)
	return cfg
}
