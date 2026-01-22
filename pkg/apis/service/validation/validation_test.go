// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	workloadidentityaws "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/aws"
	workloadidentityazure "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/azure"
	workloadidentitygcp "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/gcp"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/gardener/pkg/apis/core"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	v1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/yaml"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/validation"
)

var _ = Describe("Validation", func() {
	var (
		awsType          = "aws-route53"
		gcpType          = "google-clouddns"
		azureType        = "azure-dns"
		secretName1      = "my-secret1"
		secretName2      = "my-secret2"
		awsWLIdentName   = "my-aws-workload-identity"
		gcpWLIdentName   = "my-gcp-workload-identity"
		azureWLIdentName = "my-azure-workload-identity"
		configMapName    = "my-configmap"

		awsWorkloadIdentity   *securityv1alpha1.WorkloadIdentity
		gcpWorkloadIdentity   *securityv1alpha1.WorkloadIdentity
		azureWorkloadIdentity *securityv1alpha1.WorkloadIdentity
		valid                 = []service.DNSProvider{
			{
				Domains:    &service.DNSIncludeExclude{Include: []string{"my.domain.test"}},
				Type:       &awsType,
				SecretName: &secretName1,
			},
			{
				Type:        &awsType,
				Credentials: &secretName2,
			},
		}
		validWL = []service.DNSProvider{
			{
				Type:        &awsType,
				Credentials: &awsWLIdentName,
			},
			{
				Type:        &gcpType,
				Credentials: &gcpWLIdentName,
			},
			{
				Type:        &azureType,
				Credentials: &azureWLIdentName,
			},
		}
		resources = []core.NamedResourceReference{
			{
				Name: secretName1,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "Secret",
					Name:       "org" + secretName1,
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
			},
			{
				Name: secretName2,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "Secret",
					Name:       "org" + secretName2,
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
			},
			{
				Name: awsWLIdentName,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "WorkloadIdentity",
					Name:       "org" + awsWLIdentName,
					APIVersion: securityv1alpha1.SchemeGroupVersion.String(),
				},
			},
			{
				Name: gcpWLIdentName,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "WorkloadIdentity",
					Name:       "org" + gcpWLIdentName,
					APIVersion: securityv1alpha1.SchemeGroupVersion.String(),
				},
			},
			{
				Name: azureWLIdentName,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "WorkloadIdentity",
					Name:       "org" + azureWLIdentName,
					APIVersion: securityv1alpha1.SchemeGroupVersion.String(),
				},
			},
			{
				Name: configMapName,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "ConfigMap",
					Name:       "org" + configMapName,
					APIVersion: corev1.SchemeGroupVersion.String(),
				},
			},
		}
		resourcesIncomplete = []core.NamedResourceReference{
			{
				Name: secretName1,
			},
			{
				Name: secretName2,
			},
		}
		unsetResources []core.NamedResourceReference = nil

		makeWorkloadIdentity = func(typ string, providerConfig any) *securityv1alpha1.WorkloadIdentity {
			GinkgoHelper()

			apiVersion := typ + ".provider.extensions.gardener.cloud/v1alpha1"
			typeMeta := metav1.TypeMeta{
				APIVersion: apiVersion,
				Kind:       "WorkloadIdentityConfig",
			}
			header, err := yaml.Marshal(typeMeta)
			Expect(err).ToNot(HaveOccurred())

			content, err := yaml.Marshal(providerConfig)
			Expect(err).ToNot(HaveOccurred(), "failed to marshal provider config for type %q", typ)
			if strings.TrimSpace(string(content)) == "{}" {
				content = []byte{}
			}
			content = append(header, content...)

			return &securityv1alpha1.WorkloadIdentity{
				TypeMeta: metav1.TypeMeta{
					APIVersion: apiVersion,
					Kind:       "WorkloadIdentity",
				},
				Spec: securityv1alpha1.WorkloadIdentitySpec{
					Audiences: []string{"some-audience"},
					TargetSystem: securityv1alpha1.TargetSystem{
						Type:           typ,
						ProviderConfig: &runtime.RawExtension{Raw: content},
					},
				},
			}
		}
	)

	BeforeEach(func() {
		awsWorkloadIdentity = makeWorkloadIdentity("aws", workloadidentityaws.WorkloadIdentityConfig{RoleARN: "arn:aws:iam::123456789012:role/my-role"})
		gcpWorkloadIdentity = makeWorkloadIdentity("gcp", workloadidentitygcp.WorkloadIdentityConfig{
			ProjectID: "my-project-id",
			CredentialsConfig: &runtime.RawExtension{
				Object: &unstructured.Unstructured{
					Object: map[string]any{
						"audience":                          "//iam.googleapis.com/projects/some/audience",
						"service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/my-gardener-operator:generateAccessToken",
						"subject_token_type":                "urn:ietf:params:oauth:token-type:jwt",
						"token_url":                         "https://sts.googleapis.com/v1/token",
						"type":                              "external_account",
						"universe_domain":                   "googleapis.com",
					},
				},
			},
		})
		azureWorkloadIdentity = makeWorkloadIdentity("azure", workloadidentityazure.WorkloadIdentityConfig{
			ClientID:       "11110000-2222-3333-4444-555555555555",
			SubscriptionID: "11110001-2222-3333-4444-555555555555",
			TenantID:       "11110002-2222-3333-4444-555555555555"})
	})

	DescribeTable("#ValidateDNSConfig",
		func(config service.DNSConfig, presources *[]core.NamedResourceReference, match gomegatypes.GomegaMatcher) {
			err := validation.ValidateDNSConfig(&config, presources, nil)
			Expect(err).To(match)
		},
		Entry("empty", service.DNSConfig{}, nil, BeEmpty()),
		Entry("valid", service.DNSConfig{
			Providers: valid,
		}, &resources, BeEmpty()),
		Entry("validWL", service.DNSConfig{
			Providers: validWL,
		}, &resources, BeEmpty()),
		Entry("missing provider type", service.DNSConfig{
			Providers: modifyCopy(valid[1:], func(items []service.DNSProvider) {
				items[0].Type = nil
			}),
		}, &resources, matchers.ConsistOfFields(Fields{
			"Type":   Equal(field.ErrorTypeRequired),
			"Field":  Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].type"),
			"Detail": Equal("provider type is required"),
		})),
		Entry("invalid provider type", service.DNSConfig{
			Providers: modifyCopy(valid[1:], func(items []service.DNSProvider) {
				t := "dummy"
				items[0].Type = &t
			}),
		}, &resources, matchers.ConsistOfFields(Fields{
			"Type":     Equal(field.ErrorTypeInvalid),
			"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].type"),
			"BadValue": Equal("dummy"),
			"Detail":   Equal("unsupported provider type. Valid types are: alicloud-dns, aws-route53, azure-dns, azure-private-dns, cloudflare-dns, gdch-dns, google-clouddns, infoblox-dns, netlify-dns, openstack-designate, powerdns, remote, rfc2136"),
		})),
		Entry("missing secret name", service.DNSConfig{
			Providers: modifyCopy(valid[:1], func(items []service.DNSProvider) {
				items[0].SecretName = nil
			}),
		}, &resources, matchers.ConsistOfFields(Fields{
			"Type":   Equal(field.ErrorTypeInvalid),
			"Field":  Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName"),
			"Detail": Equal("either secretName or credentials must be provided"),
		},
			Fields{
				"Type":   Equal(field.ErrorTypeInvalid),
				"Field":  Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].credentials"),
				"Detail": Equal("either secretName or credentials must be provided"),
			})),
		Entry("missing named resource", service.DNSConfig{
			Providers: valid,
		}, &unsetResources, matchers.ConsistOfFields(
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName"),
				"BadValue": Equal("my-secret1"),
				"Detail":   Equal("secretName is not defined as named resource references at 'spec.resources'"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].credentials"),
				"BadValue": Equal("my-secret2"),
				"Detail":   Equal("credentials is not defined as named resource references at 'spec.resources'"),
			})),
		Entry("validation without considering resources", service.DNSConfig{
			Providers: valid,
		}, nil, BeEmpty()),
		Entry("secretName references workload identity", service.DNSConfig{
			Providers: []service.DNSProvider{
				{
					Type:       &awsType,
					SecretName: &awsWLIdentName,
				},
			},
		}, &resources, matchers.ConsistOfFields(
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName.kind"),
				"BadValue": Equal("WorkloadIdentity"),
				"Detail":   Equal("only kind 'Secret' resource references are allowed. To use WorkloadIdentity, please use 'credentials' field instead of 'secretName'"),
			})),
		Entry("credentials references configmap", service.DNSConfig{
			Providers: []service.DNSProvider{
				{
					Type:        &awsType,
					Credentials: &configMapName,
				},
			},
		}, &resources, matchers.ConsistOfFields(
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].credentials.kind"),
				"BadValue": Equal("ConfigMap"),
				"Detail":   Equal("only Secret or WorkloadIdentity resource references are allowed"),
			})))

	DescribeTable("#ValidateDNSConfig - with secret getter",
		func(config service.DNSConfig, presources *[]core.NamedResourceReference, getter validation.ResourceGetter, match gomegatypes.GomegaMatcher, shouldBeIgnoredIfDisabled bool) {
			err := validation.ValidateDNSConfig(&config, presources, getter)
			Expect(err).To(match)
			if shouldBeIgnoredIfDisabled {
				os.Setenv("DISABLE_SECRET_VALIDATION", "true")
				defer os.Unsetenv("DISABLE_SECRET_VALIDATION")
				err = validation.ValidateDNSConfig(&config, presources, getter)
				Expect(err).To(BeEmpty(), "validation should not fail when DISABLE_SECRET_VALIDATION is set to true")
			}
		},
		Entry("valid",
			service.DNSConfig{
				Providers: valid,
			}, &resources, secretResourceGetter(
				func(name string) (*corev1.Secret, error) {
					switch name {
					case "org" + secretName1, "org" + secretName2:
						return &corev1.Secret{
							Data: map[string][]byte{
								"accessKeyID":     []byte("myAccessKeyId"),
								"secretAccessKey": []byte("mySecretAccessKey"),
							},
						}, nil
					default:
						return nil, fmt.Errorf("unexpected secret name %q", name)
					}
				},
			), BeEmpty(), false),
		Entry("validWL",
			service.DNSConfig{
				Providers: validWL,
			}, &resources, wlResourceGetter(
				func(name string) (*securityv1alpha1.WorkloadIdentity, error) {
					switch name {
					case "org" + awsWLIdentName:
						return awsWorkloadIdentity, nil
					case "org" + gcpWLIdentName:
						return gcpWorkloadIdentity, nil
					case "org" + azureWLIdentName:
						return azureWorkloadIdentity, nil
					default:
						return nil, fmt.Errorf("unexpected workload identity name %q", name)
					}
				},
			), BeEmpty(), false),
		Entry("invalidWL",
			service.DNSConfig{
				Providers: validWL,
			}, &resources, wlResourceGetter(
				func(name string) (*securityv1alpha1.WorkloadIdentity, error) {
					switch name {
					case "org" + awsWLIdentName:
						return makeWorkloadIdentity("aws", workloadidentityaws.WorkloadIdentityConfig{}), nil
					case "org" + gcpWLIdentName:
						return makeWorkloadIdentity("gcp", workloadidentitygcp.WorkloadIdentityConfig{
							CredentialsConfig: &runtime.RawExtension{Object: &unstructured.Unstructured{Object: map[string]any{}}},
						}), nil
					case "org" + azureWLIdentName:
						return makeWorkloadIdentity("azure", workloadidentityazure.WorkloadIdentityConfig{}), nil
					default:
						return nil, fmt.Errorf("unexpected workload identity name %q", name)
					}
				},
			), ContainElements(
				matchers.HaveFields(Fields{
					"Type":     Equal(field.ErrorTypeRequired),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].credentials.ref.roleARN"),
					"BadValue": Equal(""),
					"Detail":   Equal("roleARN is required"),
				}),
				matchers.HaveFields(Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].credentials.ref.projectID"),
					"BadValue": Equal(""),
					"Detail":   Equal("does not match the expected format"),
				}),
				matchers.HaveFields(Fields{
					"Type":     Equal(field.ErrorTypeForbidden),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].credentials.ref.credentialsConfig"),
					"BadValue": Equal(""),
					"Detail":   Equal("missing required field: \"audience\""),
				}),
				matchers.HaveFields(Fields{
					"Type":     Equal(field.ErrorTypeRequired),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[2].credentials.ref.clientID"),
					"BadValue": Equal(""),
					"Detail":   Equal("clientID is required"),
				}),
			), false),
		Entry("secret data validation errors",
			service.DNSConfig{
				Providers: valid,
			}, &resources, secretResourceGetter(func(name string) (*corev1.Secret, error) {
				switch name {
				case "org" + secretName1:
					return &corev1.Secret{
						Data: map[string][]byte{
							"accessKeyID":     []byte(" myAccessKeyId"),
							"secretAccessKey": []byte("mySecretAccessKey"),
						},
					}, nil
				case "org" + secretName2:
					return &corev1.Secret{
						Data: map[string][]byte{
							"accessKeyID":     []byte("myAccessKeyId"),
							"secretAccessKey": []byte("mySecretAccessKey"),
							"wrongKey":        []byte("foo"),
						},
					}, nil
				default:
					return nil, fmt.Errorf("unexpected secret name %q", name)
				}
			}), matchers.ConsistOfFields(
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName.ref"),
					"BadValue": Equal("orgmy-secret1"),
					"Detail":   Equal("validation of secret data or provider config failed: validation failed for property accessKeyID (alias for AWS_ACCESS_KEY_ID) with value \" myAccessKeyId\": value must not contain trailing whitespace"),
				},
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].credentials.ref"),
					"BadValue": Equal("orgmy-secret2"),
					"Detail":   Equal("validation of secret data or provider config failed: validation failed for provider type aws-route53: property \"wrongKey\" is not allowed"),
				}), true),
		Entry("secret not found",
			service.DNSConfig{
				Providers: valid,
			}, &resources, secretResourceGetter(func(name string) (*corev1.Secret, error) {
				switch name {
				case "org" + secretName1:
					return &corev1.Secret{
						Data: map[string][]byte{
							"accessKeyID":     []byte("myAccessKeyId"),
							"secretAccessKey": []byte("mySecretAccessKey"),
						},
					}, nil
				case "org" + secretName2:
					return nil, fmt.Errorf("secret orgmy-secret2 not found")
				default:
					return nil, fmt.Errorf("unexpected secret name %q", name)
				}
			}), matchers.ConsistOfFields(
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].credentials.ref"),
					"BadValue": Equal("orgmy-secret2"),
					"Detail":   Equal("failed to get secret: secret orgmy-secret2 not found"),
				}), true),
		Entry("missing resource reference",
			service.DNSConfig{
				Providers: valid,
			}, &resourcesIncomplete, secretResourceGetter(func(name string) (*corev1.Secret, error) {
				switch name {
				case "org" + secretName1, "org" + secretName2:
					return &corev1.Secret{
						Data: map[string][]byte{
							"accessKeyID":     []byte("myAccessKeyId"),
							"secretAccessKey": []byte("mySecretAccessKey"),
						},
					}, nil
				default:
					return nil, fmt.Errorf("unexpected secret name %q", name)
				}
			}), matchers.ConsistOfFields(
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName"),
					"BadValue": Equal("my-secret1"),
					"Detail":   Equal("incomplete resource reference at 'spec.resources'"),
				},
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].credentials"),
					"BadValue": Equal("my-secret2"),
					"Detail":   Equal("incomplete resource reference at 'spec.resources'"),
				}), false),
		Entry("empty workload identities",
			service.DNSConfig{
				Providers: validWL,
			}, &resources, wlResourceGetter(
				func(name string) (*securityv1alpha1.WorkloadIdentity, error) {
					switch name {
					case "org" + awsWLIdentName:
						return &securityv1alpha1.WorkloadIdentity{}, nil
					case "org" + gcpWLIdentName:
						return &securityv1alpha1.WorkloadIdentity{}, nil
					case "org" + azureWLIdentName:
						return &securityv1alpha1.WorkloadIdentity{}, nil
					default:
						return nil, fmt.Errorf("unexpected workload identity name %q", name)
					}
				},
			), matchers.ConsistOfFields(
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].credentials.ref"),
					"BadValue": Equal("orgmy-aws-workload-identity"),
					"Detail":   Equal("the WorkloadIdentity resource does not contain a providerConfig"),
				},
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].credentials.ref"),
					"BadValue": Equal("orgmy-gcp-workload-identity"),
					"Detail":   Equal("the WorkloadIdentity resource does not contain a providerConfig"),
				},
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[2].credentials.ref"),
					"BadValue": Equal("orgmy-azure-workload-identity"),
					"Detail":   Equal("the WorkloadIdentity resource does not contain a providerConfig"),
				},
			), false),
	)
})

func modifyCopy(original []service.DNSProvider, modifier func([]service.DNSProvider)) []service.DNSProvider {
	var array []service.DNSProvider
	for _, p := range original {
		array = append(array, *p.DeepCopy())
	}
	modifier(array)
	return array
}

func secretResourceGetter(getter func(name string) (*corev1.Secret, error)) validation.ResourceGetter {
	return &testResourceGetter{
		secretGetter: getter,
	}
}

func wlResourceGetter(
	workloadIdentityGetter func(name string) (*securityv1alpha1.WorkloadIdentity, error),
) validation.ResourceGetter {
	return &testResourceGetter{
		workloadIdentityGetter: workloadIdentityGetter,
		internalGCPWorkloadIdentityConfig: config.InternalGCPWorkloadIdentityConfig{
			AllowedTokenURLs: []string{"https://sts.googleapis.com/v1/token"},
			AllowedServiceAccountImpersonationURLRegExps: []*regexp.Regexp{regexp.MustCompile(`^https://iamcredentials\.googleapis\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$`)},
		},
	}
}

type testResourceGetter struct {
	secretGetter                      func(name string) (*corev1.Secret, error)
	workloadIdentityGetter            func(name string) (*securityv1alpha1.WorkloadIdentity, error)
	internalGCPWorkloadIdentityConfig config.InternalGCPWorkloadIdentityConfig
}

var _ validation.ResourceGetter = &testResourceGetter{}

func (r *testResourceGetter) GetSecret(name string) (*corev1.Secret, error) {
	if r.secretGetter == nil {
		return nil, fmt.Errorf("secret getter not set")
	}
	return r.secretGetter(name)
}

func (r *testResourceGetter) GetWorkloadIdentity(name string) (*securityv1alpha1.WorkloadIdentity, error) {
	if r.workloadIdentityGetter == nil {
		return nil, fmt.Errorf("workloadIdentity getter not set")
	}
	return r.workloadIdentityGetter(name)
}

func (r *testResourceGetter) GetInternalGCPWorkloadIdentityConfig() config.InternalGCPWorkloadIdentityConfig {
	return r.internalGCPWorkloadIdentityConfig
}
