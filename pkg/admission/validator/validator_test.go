// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validator_test

import (
	"context"
	"regexp"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	gardencore "github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/apis/core/install"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gomegatypes "github.com/onsi/gomega/types"
	v1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/yaml"

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
		dnsConfigGoodWorkloadIdentity = []byte(`apiVersion: service.dns.extensions.gardener.cloud/v1alpha1
kind: DNSConfig
providers:
- credentials: shoot-dns-service-my-gcp-wl-good
  type: google-clouddns
syncProvidersFromShootSpecDNS: false
`)
		dnsConfigBadWorkloadIdentity = []byte(`apiVersion: service.dns.extensions.gardener.cloud/v1alpha1
kind: DNSConfig
providers:
- credentials: shoot-dns-service-my-gcp-wl-bad
  type: google-clouddns
syncProvidersFromShootSpecDNS: false
`)
		dnsConfigBad = []byte(`apiVersion: service.dns.extensions.gardener.cloud/v1alpha1
kind: DNSConfig
providers:
- secretName: shoot-dns-service-my-secret-bad
  type: aws-route53
syncProvidersFromShootSpecDNS: false
`)
		ctx                    = context.Background()
		secretNameGood         = "my-secret-good"
		secretNameBad          = "my-secret-bad"
		secretMappedNameGood   = "shoot-dns-service-my-secret-good"
		secretMappedNameBad    = "shoot-dns-service-my-secret-bad"
		secretNameGoodWL       = "my-gcp-wl-good"
		secretNameBadWL        = "my-gcp-wl-bad"
		secretMappedNameGoodWL = "shoot-dns-service-my-gcp-wl-good"
		secretMappedNameBadWL  = "shoot-dns-service-my-gcp-wl-bad"
		shootFunc              = func(raw []byte) *gardencore.Shoot {
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
						{
							Name: secretMappedNameGoodWL,
							ResourceRef: v1.CrossVersionObjectReference{
								Kind:       "WorkloadIdentity",
								Name:       secretNameGoodWL,
								APIVersion: "security.gardener.cloud/v1alpha1",
							},
						},
						{
							Name: secretMappedNameBadWL,
							ResourceRef: v1.CrossVersionObjectReference{
								Kind:       "WorkloadIdentity",
								Name:       secretNameBadWL,
								APIVersion: "security.gardener.cloud/v1alpha1",
							},
						},
					},
				},
			}
		}
		wlProviderConfigGood = `
apiVersion: gcp.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
projectID: "foo-valid"
credentialsConfig:
  "universe_domain": "googleapis.com"
  "type": "external_account"
  "audience": "//iam.googleapis.com/projects/11111111/locations/global/workloadIdentityPools/foopool/providers/fooprovider"
  "subject_token_type": "urn:ietf:params:oauth:token-type:jwt"
  "token_url": "https://sts.googleapis.com/v1/token"
  "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/foo@bar.example:generateAccessToken"
`
		wlProviderConfigBad = `
apiVersion: gcp.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
projectID: "foo-valid"
credentialsConfig:
  "universe_domain": "googleapis.com"
  "type": "external_account"
  "audience": "//iam.googleapis.com/projects/11111111/locations/global/workloadIdentityPools/foopool/providers/fooprovider"
  "subject_token_type": "urn:ietf:params:oauth:token-type:jwt"
  "token_url": "https://sts.foreign.com/v1/token"
  "service_account_impersonation_url": "https://iamcredentials.foreign.com/v1/projects/-/serviceAccounts/foo@bar.example:generateAccessToken"
`

		createWorkloadIdentity = func(namespace, name string, providerConfigRaw string) *securityv1alpha1.WorkloadIdentity {
			obj := &unstructured.Unstructured{}
			Expect(yaml.Unmarshal([]byte(providerConfigRaw), &obj.Object)).To(Succeed())
			return &securityv1alpha1.WorkloadIdentity{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Spec: securityv1alpha1.WorkloadIdentitySpec{
					Audiences: []string{"projects/123456789/workloadIdentity/blabla"},
					TargetSystem: securityv1alpha1.TargetSystem{
						Type: "gcp",
						ProviderConfig: &runtime.RawExtension{
							Object: obj,
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
		utilruntime.Must(securityv1alpha1.AddToScheme(mgrScheme))

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
		Expect(fakeClient.Create(ctx, createWorkloadIdentity("test", secretNameGoodWL, wlProviderConfigGood))).To(Succeed())
		Expect(fakeClient.Create(ctx, createWorkloadIdentity("test", secretNameBadWL, wlProviderConfigBad))).To(Succeed())
		validator = admissionvalidator.NewShootValidator(mgr, config.InternalGCPWorkloadIdentityConfig{
			AllowedTokenURLs: []string{"https://sts.googleapis.com/v1/token", "https://sts.googleapis.com/v1/token/new"},
			AllowedServiceAccountImpersonationURLRegExps: []*regexp.Regexp{regexp.MustCompile(`^https://iamcredentials\.googleapis\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$`)},
		})
	})

	DescribeTable("#Validate",
		func(newShoot, oldShoot *gardencore.Shoot, match gomegatypes.GomegaMatcher) {
			err := validator.Validate(ctx, newShoot, oldShoot)
			Expect(err).To(match)
		},

		Entry("create good", shootFunc(dnsConfigGood), nil, Succeed()),
		Entry("update unchanged good", shootFunc(dnsConfigGood), shootFunc(dnsConfigGood), Succeed()),
		Entry("create bad", shootFunc(dnsConfigBad), nil,
			MatchError("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName.ref: Invalid value: \"my-secret-bad\": validation of secret data or provider config failed: validation failed for provider type aws-route53: property \"badKey\" is not allowed")),
		Entry("update unchanged bad", shootFunc(dnsConfigBad), shootFunc(dnsConfigBad), Succeed()), // avoid blocking of shoot update if dnsConfig is unchanged, but validation of secret would fail because of changed data
		Entry("update good to bad", shootFunc(dnsConfigBad), shootFunc(dnsConfigGood),
			MatchError("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName.ref: Invalid value: \"my-secret-bad\": validation of secret data or provider config failed: validation failed for provider type aws-route53: property \"badKey\" is not allowed")),
		Entry("update bad to good", shootFunc(dnsConfigGood), shootFunc(dnsConfigBad), Succeed()),
		Entry("create good provider with workload identity", shootFunc(dnsConfigGoodWorkloadIdentity), nil, Succeed()),
		Entry("create bad provider with workload identity", shootFunc(dnsConfigBadWorkloadIdentity), nil,
			MatchError(SatisfyAll(
				ContainSubstring("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].credentials.ref.credentialsConfig.token_url: Forbidden: allowed values are [\"https://sts.googleapis.com/v1/token\" \"https://sts.googleapis.com/v1/token/new\""),
				ContainSubstring("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].credentials.ref.credentialsConfig.service_account_impersonation_url: Invalid value: \"https://iamcredentials.foreign.com/v1/projects/-/serviceAccounts/foo@bar.example:generateAccessToken\": should match one of the allowed regular expressions: ^https://iamcredentials\\.googleapis\\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$]"),
			))),
	)
})
