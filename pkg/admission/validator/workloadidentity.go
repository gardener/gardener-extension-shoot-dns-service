// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validator

import (
	"context"
	"fmt"

	workloadidentityaws "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/aws"
	workloadidentityazure "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/azure"
	workloadidentitygcp "github.com/gardener/external-dns-management/pkg/apis/dns/workloadidentity/gcp"
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

type workloadIdentity struct {
	gcpConfig config.InternalGCPWorkloadIdentityConfig
}

// NewWorkloadIdentityValidator returns a new instance of a WorkloadIdentity validator.
func NewWorkloadIdentityValidator(gcpConfig config.InternalGCPWorkloadIdentityConfig) extensionswebhook.Validator {
	return &workloadIdentity{
		gcpConfig: gcpConfig,
	}
}

// Validate checks whether the given new workloadidentity contains a valid AWS/Azure/Google configuration.
// Unsupported target system types are skipped.
func (wi *workloadIdentity) Validate(_ context.Context, newObj, oldObj client.Object) error {
	workloadIdentity, ok := newObj.(*securityv1alpha1.WorkloadIdentity)
	if !ok {
		return fmt.Errorf("wrong object type %T", newObj)
	}

	var oldWorkloadIdentity *securityv1alpha1.WorkloadIdentity
	if oldObj != nil {
		oldWorkloadIdentity, ok = oldObj.(*securityv1alpha1.WorkloadIdentity)
		if !ok {
			return fmt.Errorf("wrong object type %T for old object", oldObj)
		}
	}

	switch workloadIdentity.Spec.TargetSystem.Type {
	case "aws":
		return wi.validateAWS(workloadIdentity, oldWorkloadIdentity)
	case "azure":
		return wi.validateAzure(workloadIdentity, oldWorkloadIdentity)
	case "gcp":
		return wi.validateGCP(workloadIdentity, oldWorkloadIdentity)
	default:
		// Skip validation for unsupported target system types
		return nil
	}
}

func (wi *workloadIdentity) validateAWS(newObj, oldObj *securityv1alpha1.WorkloadIdentity) error {
	newConfig, err := awsConfigFromRawExtension("new", newObj.Spec.TargetSystem.ProviderConfig)
	if err != nil {
		return err
	}

	fieldPath := field.NewPath("spec", "targetSystem", "providerConfig")
	if oldObj != nil {
		oldConfig, err := awsConfigFromRawExtension("old", oldObj.Spec.TargetSystem.ProviderConfig)
		if err != nil {
			return err
		}

		errList := workloadidentityaws.ValidateWorkloadIdentityConfigUpdate(oldConfig, newConfig, fieldPath)
		if len(errList) > 0 {
			return fmt.Errorf("validation of target system's configuration failed: %w", errList.ToAggregate())
		}
		return nil
	}

	errList := workloadidentityaws.ValidateWorkloadIdentityConfig(newConfig, fieldPath)
	if len(errList) > 0 {
		return fmt.Errorf("validation of target system's configuration failed: %w", errList.ToAggregate())
	}
	return nil
}

func (wi *workloadIdentity) validateAzure(newObj, oldObj *securityv1alpha1.WorkloadIdentity) error {
	newConfig, err := azureConfigFromRawExtension("new", newObj.Spec.TargetSystem.ProviderConfig)
	if err != nil {
		return err
	}

	fieldPath := field.NewPath("spec", "targetSystem", "providerConfig")
	if oldObj != nil {
		oldConfig, err := azureConfigFromRawExtension("old", oldObj.Spec.TargetSystem.ProviderConfig)
		if err != nil {
			return err
		}

		errList := workloadidentityazure.ValidateWorkloadIdentityConfigUpdate(oldConfig, newConfig, fieldPath)
		if len(errList) > 0 {
			return fmt.Errorf("validation of target system's configuration failed: %w", errList.ToAggregate())
		}
		return nil
	}

	errList := workloadidentityazure.ValidateWorkloadIdentityConfig(newConfig, fieldPath)
	if len(errList) > 0 {
		return fmt.Errorf("validation of target system's configuration failed: %w", errList.ToAggregate())
	}
	return nil
}

func (wi *workloadIdentity) validateGCP(newObj, oldObj *securityv1alpha1.WorkloadIdentity) error {
	newConfig, err := gcpConfigFromRawExtension("new", newObj.Spec.TargetSystem.ProviderConfig)
	if err != nil {
		return err
	}

	fieldPath := field.NewPath("spec", "targetSystem", "providerConfig")
	if oldObj != nil {
		oldConfig, err := gcpConfigFromRawExtension("old", oldObj.Spec.TargetSystem.ProviderConfig)
		if err != nil {
			return err
		}

		errList := workloadidentitygcp.ValidateWorkloadIdentityConfigUpdate(oldConfig, newConfig, fieldPath, wi.gcpConfig.AllowedTokenURLs, wi.gcpConfig.AllowedServiceAccountImpersonationURLRegExps)
		if len(errList) > 0 {
			return fmt.Errorf("validation of target system's configuration failed: %w", errList.ToAggregate())
		}
		return nil
	}

	errList := workloadidentitygcp.ValidateWorkloadIdentityConfig(newConfig, fieldPath, wi.gcpConfig.AllowedTokenURLs, wi.gcpConfig.AllowedServiceAccountImpersonationURLRegExps)
	if len(errList) > 0 {
		return fmt.Errorf("validation of target system's configuration failed: %w", errList.ToAggregate())
	}
	return nil
}

func awsConfigFromRawExtension(name string, providerConfig *runtime.RawExtension) (*workloadidentityaws.WorkloadIdentityConfig, error) {
	cfg := &workloadidentityaws.WorkloadIdentityConfig{}
	if err := configFromRawExtension(name, "AWS", providerConfig, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func azureConfigFromRawExtension(name string, providerConfig *runtime.RawExtension) (*workloadidentityazure.WorkloadIdentityConfig, error) {
	cfg := &workloadidentityazure.WorkloadIdentityConfig{}
	if err := configFromRawExtension(name, "Azure", providerConfig, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func gcpConfigFromRawExtension(name string, providerConfig *runtime.RawExtension) (*workloadidentitygcp.WorkloadIdentityConfig, error) {
	cfg := &workloadidentitygcp.WorkloadIdentityConfig{}
	if err := configFromRawExtension(name, "Google", providerConfig, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func configFromRawExtension(name, infraType string, providerConfig *runtime.RawExtension, cfg any) error {
	if providerConfig == nil || len(providerConfig.Raw) == 0 {
		return fmt.Errorf("the %s target system is missing the %s providerConfig configuration", name, infraType)
	}
	if err := yaml.Unmarshal(providerConfig.Raw, cfg); err != nil {
		return fmt.Errorf("failed to unmarshal %s %s workload identity config: %w", infraType, name, err)
	}
	return nil
}
