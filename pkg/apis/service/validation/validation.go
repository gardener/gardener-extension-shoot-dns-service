// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"strings"

	"github.com/gardener/gardener/pkg/apis/core"
	"k8s.io/apimachinery/pkg/util/validation/field"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	service2 "github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

var supportedProviderTypes = []string{
	"alicloud-dns",
	"aws-route53",
	"azure-dns", "azure-private-dns",
	"cloudflare-dns",
	"google-clouddns",
	"infoblox-dns",
	"netlify-dns",
	"openstack-designate",
	"powerdns",
	"remote",
	"rfc2136",
}

// ValidateDNSConfig validates the passed DNSConfig.
// If resources != nil, it also validates if the referenced secrets are defined.
func ValidateDNSConfig(config *service.DNSConfig, resources *[]core.NamedResourceReference) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(config.Providers) > 0 {
		allErrs = append(allErrs, validateProviders(config.Providers, resources)...)
	}
	return allErrs
}

func validateProviders(providers []service.DNSProvider, presources *[]core.NamedResourceReference) field.ErrorList {
	allErrs := field.ErrorList{}
	path := field.NewPath("spec", "extensions", "[@.type='"+service2.ExtensionType+"']", "providerConfig")
	for i, p := range providers {
		if p.Type == nil || *p.Type == "" {
			allErrs = append(allErrs, field.Required(path.Index(i).Child("type"), "provider type is required"))
		} else if !isSupportedProviderType(*p.Type) {
			allErrs = append(allErrs, field.Invalid(path.Index(i).Child("type"), *p.Type,
				fmt.Sprintf("unsupported provider type. Valid types are: %s", strings.Join(supportedProviderTypes, ", "))))
		}
		if p.SecretName == nil || *p.SecretName == "" {
			allErrs = append(allErrs, field.Required(path.Index(i).Child("secretName"), "secret name is required"))
		} else if presources != nil {
			found := false
			for _, ref := range *presources {
				if ref.Name == *p.SecretName {
					found = true
					break
				}
			}
			if !found {
				allErrs = append(allErrs, field.Invalid(path.Index(i).Child("secretName"), *p.SecretName, "secret name is not defined as named resource references at 'spec.resources'"))
			}
		}
	}
	return allErrs
}

func isSupportedProviderType(providerType string) bool {
	for _, typ := range supportedProviderTypes {
		if typ == providerType {
			return true
		}
	}
	return false
}
