// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validation

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/resources"
	workloadidentityaws "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/aws"
	workloadidentityazure "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/azure"
	workloadidentitygcp "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/gcp"
	compoundvalidation "github.com/gardener/external-dns-management/pkg/controller/provider/compound/validation"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"github.com/gardener/gardener/pkg/apis/core"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/yaml"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	service2 "github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

var supportedProviderTypes = []string{
	"alicloud-dns",
	"aws-route53",
	"azure-dns", "azure-private-dns",
	"cloudflare-dns",
	"gdch-dns",
	"google-clouddns",
	"infoblox-dns",
	"netlify-dns",
	"openstack-designate",
	"powerdns",
	"remote",
	"rfc2136",
}

// ResourceGetter is an interface that defines methods to get Kubernetes resources from the Garden cluster.
type ResourceGetter interface {
	// GetSecret retrieves a Kubernetes Secret by its name.
	GetSecret(name string) (*corev1.Secret, error)
	// GetWorkloadIdentity retrieves a WorkloadIdentity by its name.
	GetWorkloadIdentity(name string) (*securityv1alpha1.WorkloadIdentity, error)
	// GetInternalGCPWorkloadIdentityConfig returns the internal GCP Workload Identity configuration.
	GetInternalGCPWorkloadIdentityConfig() config.InternalGCPWorkloadIdentityConfig
}

// ValidateDNSConfig validates the passed DNSConfig.
// If resources != nil, it also validates if the referenced secrets are defined.
func ValidateDNSConfig(config *service.DNSConfig, resources *[]core.NamedResourceReference, getter ResourceGetter) field.ErrorList {
	allErrs := field.ErrorList{}

	if len(config.Providers) > 0 {
		allErrs = append(allErrs, validateProviders(config.Providers, resources, getter)...)
	}
	return allErrs
}

func validateProviders(providers []service.DNSProvider, presources *[]core.NamedResourceReference, getter ResourceGetter) field.ErrorList {
	allErrs := field.ErrorList{}
	path := field.NewPath("spec", "extensions", "[@.type='"+service2.ExtensionType+"']", "providerConfig")
	for i, p := range providers {
		if p.Type == nil || *p.Type == "" {
			allErrs = append(allErrs, field.Required(path.Index(i).Child("type"), "provider type is required"))
		} else if !isSupportedProviderType(*p.Type) {
			allErrs = append(allErrs, field.Invalid(path.Index(i).Child("type"), *p.Type,
				fmt.Sprintf("unsupported provider type. Valid types are: %s", strings.Join(supportedProviderTypes, ", "))))
		}
		secretName := ptr.Deref(p.SecretName, "")
		credentials := ptr.Deref(p.Credentials, "")
		if secretName == "" && credentials == "" {
			allErrs = append(allErrs, field.Invalid(path.Index(i).Child("secretName"), "", "either secretName or credentials must be provided"))
			allErrs = append(allErrs, field.Invalid(path.Index(i).Child("credentials"), "", "either secretName or credentials must be provided"))
		} else if secretName != "" && credentials != "" {
			allErrs = append(allErrs, field.Invalid(path.Index(i).Child("secretName"), secretName, "only one of secretName or credentials must be provided"))
			allErrs = append(allErrs, field.Invalid(path.Index(i).Child("credentials"), credentials, "only one of secretName or credentials must be provided"))
		} else if presources != nil {
			child := path.Index(i).Child("secretName")
			refName := secretName
			fieldName := "secretName"
			allowWorkloadIdentity := false
			if credentials != "" {
				child = path.Index(i).Child("credentials")
				refName = credentials
				fieldName = "credentials"
				allowWorkloadIdentity = true
			}
			var credentialRef core.NamedResourceReference
			for _, ref := range *presources {
				if ref.Name == refName {
					credentialRef = ref
					break
				}
			}
			if credentialRef.Name == "" {
				allErrs = append(allErrs, field.Invalid(child, refName, fieldName+" is not defined as named resource references at 'spec.resources'"))
				continue // skip validation if no resources are defined
			}
			if credentialRef.ResourceRef.Name == "" {
				allErrs = append(allErrs, field.Invalid(child, refName, "incomplete resource reference at 'spec.resources'"))
				continue
			}
			validateProviderSecretOrWorkloadIdentity(credentialRef, allowWorkloadIdentity, ptr.Deref(p.Type, ""), path.Index(i), child, getter, &allErrs)
		}
	}
	return allErrs
}

func isSupportedProviderType(providerType string) bool {
	return slices.Contains(supportedProviderTypes, providerType)
}

func validateProviderSecretOrWorkloadIdentity(resourceRef core.NamedResourceReference, allowWorkloadIdentity bool, providerType string, path, secretChild *field.Path, getter ResourceGetter, allErrs *field.ErrorList) {
	switch {
	case resourceRef.ResourceRef.Kind == "Secret" && resourceRef.ResourceRef.APIVersion == "v1":
		validateProviderSecret(resourceRef.ResourceRef.Name, providerType, path, secretChild, getter, allErrs)
	case resourceRef.ResourceRef.Kind == "WorkloadIdentity" && resourceRef.ResourceRef.APIVersion == securityv1alpha1.SchemeGroupVersion.String():
		if !allowWorkloadIdentity {
			*allErrs = append(*allErrs, field.Invalid(secretChild.Child("kind"), resourceRef.ResourceRef.Kind, "only kind 'Secret' resource references are allowed. To use WorkloadIdentity, please use 'credentials' field instead of 'secretName'"))
			return
		}
		validateProviderWorkloadIdentity(resourceRef.ResourceRef.Name, providerType, path, secretChild, getter, allErrs)
	default:
		*allErrs = append(*allErrs, field.Invalid(secretChild.Child("kind"), resourceRef.ResourceRef.Kind, "only Secret or WorkloadIdentity resource references are allowed"))
	}
}

func validateProviderSecret(secretName, providerType string, path, secretChild *field.Path, getter ResourceGetter, allErrs *field.ErrorList) {
	if os.Getenv("DISABLE_SECRET_VALIDATION") == "true" {
		return
	}
	if providerType != "" && getter != nil {
		secret, err := getter.GetSecret(secretName)
		if err != nil {
			*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), secretName, fmt.Sprintf("failed to get secret: %s", err)))
			return
		}
		adapter, err := getDNSHandlerAdapter(providerType)
		if err != nil {
			*allErrs = append(*allErrs, field.Invalid(path.Child("type"), providerType, fmt.Sprintf("failed to get DNSHandlerAdapter: %s", err)))
			return
		}
		props := resources.GetSecretPropertiesFrom(secret)
		if err := adapter.ValidateCredentialsAndProviderConfig(props, nil); err != nil {
			*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), secretName, fmt.Sprintf("validation of secret data or provider config failed: %s", err)))
		}
	}
}

func validateProviderWorkloadIdentity(workloadIdentityName, providerType string, path, secretChild *field.Path, getter ResourceGetter, allErrs *field.ErrorList) {
	if providerType != "" && getter != nil {
		workloadIdentity, err := getter.GetWorkloadIdentity(workloadIdentityName)
		if err != nil {
			*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), workloadIdentityName,
				fmt.Sprintf("failed to get the WorkloadIdentity resource: %s", err)))
			return
		}
		if workloadIdentity.Spec.TargetSystem.ProviderConfig == nil || workloadIdentity.Spec.TargetSystem.ProviderConfig.Raw == nil {
			*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), workloadIdentityName,
				"the WorkloadIdentity resource does not contain a providerConfig"))
			return
		}

		switch providerType {
		case "aws-route53":
			if workloadIdentity.Spec.TargetSystem.Type != "aws" {
				*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), workloadIdentityName,
					"the WorkloadIdentity provider must be 'aws' for AWS Route53 provider"))
				return
			}

			var providerConfig workloadidentityaws.WorkloadIdentityConfig
			if err := yaml.Unmarshal(workloadIdentity.Spec.TargetSystem.ProviderConfig.Raw, &providerConfig); err != nil {
				*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), workloadIdentityName,
					fmt.Sprintf("failed to unmarshal the WorkloadIdentity providerConfig: %s", err)))
				return
			}

			errList := workloadidentityaws.ValidateWorkloadIdentityConfig(&providerConfig, secretChild.Child("ref"))
			if len(errList) > 0 {
				*allErrs = append(*allErrs, errList...)
				return
			}
		case "google-clouddns":
			if workloadIdentity.Spec.TargetSystem.Type != "gcp" {
				*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), workloadIdentityName,
					"the WorkloadIdentity provider must be 'gcp' for Google CloudDNS provider"))
				return
			}

			var providerConfig workloadidentitygcp.WorkloadIdentityConfig
			if err := yaml.Unmarshal(workloadIdentity.Spec.TargetSystem.ProviderConfig.Raw, &providerConfig); err != nil {
				*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), workloadIdentityName,
					fmt.Sprintf("failed to unmarshal the WorkloadIdentity providerConfig: %s", err)))
				return
			}

			gcpConfig := getter.GetInternalGCPWorkloadIdentityConfig()
			errList := workloadidentitygcp.ValidateWorkloadIdentityConfig(&providerConfig, secretChild.Child("ref"), gcpConfig.AllowedTokenURLs, gcpConfig.AllowedServiceAccountImpersonationURLRegExps)
			if len(errList) > 0 {
				*allErrs = append(*allErrs, errList...)
				return
			}
		case "azure-dns", "azure-private-dns":
			if workloadIdentity.Spec.TargetSystem.Type != "azure" {
				*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), workloadIdentityName,
					"the WorkloadIdentity provider must be 'azure' for Azure DNS providers"))
				return
			}

			var providerConfig workloadidentityazure.WorkloadIdentityConfig
			if err := yaml.Unmarshal(workloadIdentity.Spec.TargetSystem.ProviderConfig.Raw, &providerConfig); err != nil {
				*allErrs = append(*allErrs, field.Invalid(secretChild.Child("ref"), workloadIdentityName,
					fmt.Sprintf("failed to unmarshal the WorkloadIdentity providerConfig: %s", err)))
				return
			}

			errList := workloadidentityazure.ValidateWorkloadIdentityConfig(&providerConfig, secretChild.Child("ref"))
			if len(errList) > 0 {
				*allErrs = append(*allErrs, errList...)
				return
			}
		default:
			*allErrs = append(*allErrs, field.Invalid(path.Child("type"), providerType,
				fmt.Sprintf("WorkloadIdentity is not supported for provider type %q", providerType)))
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
