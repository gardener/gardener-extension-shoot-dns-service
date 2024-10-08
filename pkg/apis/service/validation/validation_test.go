// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation_test

import (
	"github.com/gardener/gardener/pkg/apis/core"
	"github.com/gardener/gardener/pkg/utils/test/matchers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	gomegatypes "github.com/onsi/gomega/types"
	v1 "k8s.io/api/autoscaling/v1"
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
				Name:        secretName1,
				ResourceRef: v1.CrossVersionObjectReference{},
			},
			{
				Name:        secretName2,
				ResourceRef: v1.CrossVersionObjectReference{},
			},
		}
		resources2 = []core.NamedResourceReference{
			{
				Name:        secretName2,
				ResourceRef: v1.CrossVersionObjectReference{},
			},
		}
		unsetResources []core.NamedResourceReference = nil
	)

	DescribeTable("#ValidateDNSConfig",
		func(config service.DNSConfig, presources *[]core.NamedResourceReference, match gomegatypes.GomegaMatcher) {
			err := validation.ValidateDNSConfig(&config, presources)
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
			"Detail":   Equal("unsupported provider type. Valid types are: alicloud-dns, aws-route53, azure-dns, azure-private-dns, cloudflare-dns, google-clouddns, infoblox-dns, netlify-dns, openstack-designate, powerdns, remote, rfc2136"),
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
})

func modifyCopy(orginal []service.DNSProvider, modifier func([]service.DNSProvider)) []service.DNSProvider {
	var array []service.DNSProvider
	for _, p := range orginal {
		array = append(array, *p.DeepCopy())
	}
	modifier(array)
	return array
}
