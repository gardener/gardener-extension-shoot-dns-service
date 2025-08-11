// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validator_test

import (
	"context"

	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	gardencore "github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apis/core/install"
	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	v1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	admissionvalidator "github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/validator"
	serviceinstall "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/install"
)

var _ = Describe("Shoot Validator", func() {
	var (
		mgrScheme  *runtime.Scheme
		fakeClient client.Client
		mgr        manager.Manager

		validator     extensionswebhook.Validator
		dnsConfigGood = []byte(`apiVersion: service.dns.extensions.gardener.cloud/v1alpha1
kind: DNSConfig
providers:
- secretName: shoot-dns-service-my-secret-good
  type: aws-route53
syncProvidersFromShootSpecDNS: false
`)
		dnsConfigBad = []byte(`apiVersion: service.dns.extensions.gardener.cloud/v1alpha1
kind: DNSConfig
providers:
- secretName: shoot-dns-service-my-secret-bad
  type: aws-route53
syncProvidersFromShootSpecDNS: false
`)
		ctx                  = context.Background()
		secretNameGood       = "my-secret-good"
		secretNameBad        = "my-secret-bad"
		secretMappedNameGood = "shoot-dns-service-my-secret-good"
		secretMappedNameBad  = "shoot-dns-service-my-secret-bad"
		shootFunc            = func(raw []byte) *gardencore.Shoot {
			return &gardencore.Shoot{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shoot",
					Namespace: "test",
				},
				Spec: gardencore.ShootSpec{
					Extensions: []gardencore.Extension{
						{
							Type: "shoot-dns-service",
							ProviderConfig: &runtime.RawExtension{
								Raw: raw,
							},
						},
					},
					Resources: []gardencore.NamedResourceReference{
						{
							Name: secretMappedNameGood,
							ResourceRef: v1.CrossVersionObjectReference{
								Kind:       "Secret",
								Name:       secretNameGood,
								APIVersion: "v1",
							},
						},
						{
							Name: secretMappedNameBad,
							ResourceRef: v1.CrossVersionObjectReference{
								Kind:       "Secret",
								Name:       secretNameBad,
								APIVersion: "v1",
							},
						},
					},
				},
			}
		}
	)

	BeforeEach(func() {
		mgrScheme = runtime.NewScheme()
		install.Install(mgrScheme)
		serviceinstall.Install(mgrScheme)
		utilruntime.Must(scheme.AddToScheme(mgrScheme))

		fakeClient = fakeclient.NewClientBuilder().WithScheme(mgrScheme).Build()
		mgr = &test.FakeManager{
			Scheme: mgrScheme,
			Client: fakeClient,
		}
		Expect(fakeClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretNameGood,
				Namespace: "test",
			},
			Data: map[string][]byte{
				"accessKeyID":     []byte("myaccessKeyID"),
				"secretAccessKey": []byte("mysecretAccessKey"),
			},
		})).To(Succeed())
		Expect(fakeClient.Create(ctx, &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretNameBad,
				Namespace: "test",
			},
			Data: map[string][]byte{
				"accessKeyID":     []byte("myaccessKeyID"),
				"secretAccessKey": []byte("mysecretAccessKey"),
				"badKey":          []byte("mybadKey"),
			},
		})).To(Succeed())
		validator = admissionvalidator.NewShootValidator(mgr)
	})

	DescribeTable("#Validate",
		func(newShoot, oldShoot *gardencore.Shoot, match gomegatypes.GomegaMatcher) {
			err := validator.Validate(ctx, newShoot, oldShoot)
			Expect(err).To(match)
		},

		Entry("create good", shootFunc(dnsConfigGood), nil, Succeed()),
		Entry("update unchanged good", shootFunc(dnsConfigGood), shootFunc(dnsConfigGood), Succeed()),
		Entry("create bad", shootFunc(dnsConfigBad), nil, MatchError("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName.ref: Invalid value: \"my-secret-bad\": validation of secret data or provider config failed: validation failed for provider type aws-route53: property \"badKey\" is not allowed")),
		Entry("update unchanged bad", shootFunc(dnsConfigBad), shootFunc(dnsConfigBad), Succeed()), // avoid blocking of shoot update if dnsConfig is unchanged, but validation of secret would fail because of changed data
		Entry("update good to bad", shootFunc(dnsConfigBad), shootFunc(dnsConfigGood), MatchError("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName.ref: Invalid value: \"my-secret-bad\": validation of secret data or provider config failed: validation failed for provider type aws-route53: property \"badKey\" is not allowed")),
		Entry("update bad to good", shootFunc(dnsConfigGood), shootFunc(dnsConfigBad), Succeed()),
	)
})
