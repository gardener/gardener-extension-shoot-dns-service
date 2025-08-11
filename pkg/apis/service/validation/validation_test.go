// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"fmt"
	"os"

	"github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	v1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/validation"
)

var _ = Describe("Validation", func() {
	var (
		awsType     = "aws-route53"
		secretName1 = "my-secret1"
		secretName2 = "my-secret2"
		valid       = []service.DNSProvider{
			{
				Domains:    &service.DNSIncludeExclude{Include: []string{"my.domain.test"}},
				Type:       &awsType,
				SecretName: &secretName1,
			},
			{
				Type:       &awsType,
				SecretName: &secretName2,
			},
		}
		resources = []core.NamedResourceReference{
			{
				Name: secretName1,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "Secret",
					Name:       "org" + secretName1,
					APIVersion: core.SchemeGroupVersion.String(),
				},
			},
			{
				Name: secretName2,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "Secret",
					Name:       "org" + secretName2,
					APIVersion: core.SchemeGroupVersion.String(),
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
		resources2 = []core.NamedResourceReference{
			{
				Name: secretName2,
				ResourceRef: v1.CrossVersionObjectReference{
					Kind:       "Secret",
					Name:       "org" + secretName2,
					APIVersion: core.SchemeGroupVersion.String(),
				},
			},
		}
		unsetResources []core.NamedResourceReference = nil
	)

	DescribeTable("#ValidateDNSConfig",
		func(config service.DNSConfig, presources *[]core.NamedResourceReference, match gomegatypes.GomegaMatcher) {
			err := validation.ValidateDNSConfig(&config, presources, nil)
			Expect(err).To(match)
		},
		Entry("empty", service.DNSConfig{}, nil, BeEmpty()),
		Entry("valid", service.DNSConfig{
			Providers: valid,
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
			Providers: modifyCopy(valid[1:], func(items []service.DNSProvider) {
				items[0].SecretName = nil
			}),
		}, &resources, matchers.ConsistOfFields(Fields{
			"Type":   Equal(field.ErrorTypeRequired),
			"Field":  Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName"),
			"Detail": Equal("secret name is required"),
		})),
		Entry("missing named resource", service.DNSConfig{
			Providers: valid,
		}, &resources2, matchers.ConsistOfFields(Fields{
			"Type":     Equal(field.ErrorTypeInvalid),
			"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName"),
			"BadValue": Equal("my-secret1"),
			"Detail":   Equal("secret name is not defined as named resource references at 'spec.resources'"),
		})),
		Entry("missing resources", service.DNSConfig{
			Providers: valid,
		}, &unsetResources, matchers.ConsistOfFields(
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName"),
				"BadValue": Equal("my-secret1"),
				"Detail":   Equal("secret name is not defined as named resource references at 'spec.resources'"),
			},
			Fields{
				"Type":     Equal(field.ErrorTypeInvalid),
				"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].secretName"),
				"BadValue": Equal("my-secret2"),
				"Detail":   Equal("secret name is not defined as named resource references at 'spec.resources'"),
			})),
		Entry("validation without considering resources", service.DNSConfig{
			Providers: valid,
		}, nil, BeEmpty()))

	DescribeTable("#ValidateDNSConfig - with secret getter",
		func(config service.DNSConfig, presources *[]core.NamedResourceReference, getter validation.SecretGetter, match gomegatypes.GomegaMatcher, shouldBeIgnoredIfDisabled bool) {
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
			}, &resources, func(name string) (*corev1.Secret, error) {
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
			}, BeEmpty(), false),
		Entry("secret data validation errors",
			service.DNSConfig{
				Providers: valid,
			}, &resources, func(name string) (*corev1.Secret, error) {
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
			}, matchers.ConsistOfFields(
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName.ref"),
					"BadValue": Equal("orgmy-secret1"),
					"Detail":   Equal("validation of secret data or provider config failed: validation failed for property accessKeyID (alias for AWS_ACCESS_KEY_ID) with value \" myAccessKeyId\": value must not contain trailing whitespace"),
				},
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].secretName.ref"),
					"BadValue": Equal("orgmy-secret2"),
					"Detail":   Equal("validation of secret data or provider config failed: validation failed for provider type aws-route53: property \"wrongKey\" is not allowed"),
				}), true),
		Entry("secret not found",
			service.DNSConfig{
				Providers: valid,
			}, &resources, func(name string) (*corev1.Secret, error) {
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
			}, matchers.ConsistOfFields(
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].secretName.ref"),
					"BadValue": Equal("orgmy-secret2"),
					"Detail":   Equal("failed to get secret: secret orgmy-secret2 not found"),
				}), true),
		Entry("missing resource reference",
			service.DNSConfig{
				Providers: valid,
			}, &resourcesIncomplete, func(name string) (*corev1.Secret, error) {
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
			}, matchers.ConsistOfFields(
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[0].secretName"),
					"BadValue": Equal("my-secret1"),
					"Detail":   Equal("incomplete resource reference at 'spec.resources'"),
				},
				Fields{
					"Type":     Equal(field.ErrorTypeInvalid),
					"Field":    Equal("spec.extensions.[@.type='shoot-dns-service'].providerConfig[1].secretName"),
					"BadValue": Equal("my-secret2"),
					"Detail":   Equal("incomplete resource reference at 'spec.resources'"),
				}), false),
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
