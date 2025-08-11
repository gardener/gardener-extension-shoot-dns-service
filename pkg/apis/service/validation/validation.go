// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"os"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/resources"
	compoundvalidation "github.com/gardener/external-dns-management/pkg/controller/provider/compound/validation"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/gardener/pkg/apis/core"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

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

// SecretGetter is a function type that reads a Kubernetes Secret by its name.
type SecretGetter func(name string) (*corev1.Secret, error)

// ValidateDNSConfig validates the passed DNSConfig.
// If resources != nil, it also validates if the referenced secrets are defined.
func ValidateDNSConfig(config *service.DNSConfig, resources *[]core.NamedResourceReference, getter SecretGetter) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(config.Providers) > 0 {
		allErrs = append(allErrs, validateProviders(config.Providers, resources, getter)...)
	}
	return allErrs
}

func validateProviders(providers []service.DNSProvider, presources *[]core.NamedResourceReference, getter SecretGetter) field.ErrorList {
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
			var (
				projectSecretName string
				found             bool
			)
			for _, ref := range *presources {
				if ref.Name == *p.SecretName {
					found = true
					projectSecretName = ref.ResourceRef.Name
					break
				}
			}
			if !found {
				allErrs = append(allErrs, field.Invalid(path.Index(i).Child("secretName"), *p.SecretName, "secret name is not defined as named resource references at 'spec.resources'"))
				continue // skip validation if no resources are defined
			}
			if projectSecretName == "" {
				allErrs = append(allErrs, field.Invalid(path.Index(i).Child("secretName"), *p.SecretName, "incomplete resource reference at 'spec.resources'"))
				continue
			}
			validateProviderSecret(projectSecretName, ptr.Deref(p.Type, ""), path.Index(i), getter, &allErrs)
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

func validateProviderSecret(secretName, providerType string, path *field.Path, getter SecretGetter, allErrs *field.ErrorList) {
	if os.Getenv("DISABLE_SECRET_VALIDATION") == "true" {
		return
	}
	if providerType != "" && getter != nil {
		secret, err := getter(secretName)
		if err != nil {
			*allErrs = append(*allErrs, field.Invalid(path.Child("secretName").Child("ref"), secretName, fmt.Sprintf("failed to get secret: %s", err)))
			return
		}
		adapter, err := getDNSHandlerAdapter(providerType)
		if err != nil {
			*allErrs = append(*allErrs, field.Invalid(path.Child("type"), providerType, fmt.Sprintf("failed to get DNSHandlerAdapter: %s", err)))
			return
		}
		props := resources.GetSecretPropertiesFrom(secret)
		if err := adapter.ValidateCredentialsAndProviderConfig(props, nil); err != nil {
			*allErrs = append(*allErrs, field.Invalid(path.Child("secretName").Child("ref"), secretName, fmt.Sprintf("validation of secret data or provider config failed: %s", err)))
		}
	}
}

func getDNSHandlerAdapter(providerType string) (provider.DNSHandlerAdapter, error) {
	adaptor := compoundvalidation.GetAdaptor(providerType)
	if adaptor != nil {
		return adaptor, nil
	}
	return nil, fmt.Errorf("no DNSHandlerAdapter found for provider type %q", providerType)
}
