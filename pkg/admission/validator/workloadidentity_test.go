// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package validator_test

import (
	"context"
	"regexp"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	securityv1alpha1 "github.com/gardener/gardener/pkg/apis/security/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/admission/validator"
)

var _ = Describe("WorkloadIdentity validator", func() {
	Describe("#Validate", func() {
		var (
			ctx                       = context.Background()
			workloadIdentityValidator = validator.NewWorkloadIdentityValidator(config.InternalGCPWorkloadIdentityConfig{
				AllowedTokenURLs: []string{"https://sts.googleapis.com/v1/token", "https://sts.googleapis.com/v1/token/new"},
				AllowedServiceAccountImpersonationURLRegExps: []*regexp.Regexp{regexp.MustCompile(`^https://iamcredentials\.googleapis\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$`)},
			})
			workloadIdentity *securityv1alpha1.WorkloadIdentity
		)

		It("should skip validation if workload identity is not of type 'aws'", func() {
			wi := &securityv1alpha1.WorkloadIdentity{
				Spec: securityv1alpha1.WorkloadIdentitySpec{
					Audiences: []string{"foo"},
					TargetSystem: securityv1alpha1.TargetSystem{
						Type: "foo",
					},
				},
			}
			Expect(workloadIdentityValidator.Validate(ctx, wi, nil)).To(Succeed())
		})

		Context("AWS Workload Identity", func() {
			BeforeEach(func() {
				workloadIdentity = &securityv1alpha1.WorkloadIdentity{
					Spec: securityv1alpha1.WorkloadIdentitySpec{
						Audiences: []string{"foo"},
						TargetSystem: securityv1alpha1.TargetSystem{
							Type: "aws",
							ProviderConfig: &runtime.RawExtension{
								Raw: []byte(`
apiVersion: aws.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
roleARN: "foo"
`),
							},
						},
					},
				}
			})

			It("should successfully validate the creation of a workload identity", func() {
				Expect(workloadIdentityValidator.Validate(ctx, workloadIdentity, nil)).To(Succeed())
			})

			It("should successfully validate the update of a workload identity", func() {
				newWorkloadIdentity := workloadIdentity.DeepCopy()
				newWorkloadIdentity.Spec.TargetSystem.ProviderConfig.Raw = []byte(`
apiVersion: aws.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
roleARN: "foo2"
`)
				Expect(workloadIdentityValidator.Validate(ctx, newWorkloadIdentity, workloadIdentity)).To(Succeed())
			})

			It("should fail to validate if roleARN is empty", func() {
				workloadIdentity.Spec.TargetSystem.ProviderConfig.Raw = []byte(`
apiVersion: aws.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
`)
				err := workloadIdentityValidator.Validate(ctx, workloadIdentity, nil)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validation of target system's configuration failed: spec.targetSystem.providerConfig.roleARN: Required value: roleARN is required"))
			})
		})

		Context("Azure Workload Identity", func() {
			BeforeEach(func() {
				workloadIdentity = &securityv1alpha1.WorkloadIdentity{
					Spec: securityv1alpha1.WorkloadIdentitySpec{
						Audiences: []string{"foo"},
						TargetSystem: securityv1alpha1.TargetSystem{
							Type: "azure",
							ProviderConfig: &runtime.RawExtension{
								Raw: []byte(`
apiVersion: azure.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
clientID: "11111c4e-db61-17fa-a141-ed39b34aa561"
tenantID: "22222c4e-db61-17fa-a141-ed39b34aa561"
subscriptionID: "33333c4e-db61-17fa-a141-ed39b34aa561"
`),
							},
						},
					},
				}
			})

			It("should successfully validate the creation of a workload identity", func() {
				Expect(workloadIdentityValidator.Validate(ctx, workloadIdentity, nil)).To(Succeed())
			})

			It("should successfully validate the update of a workload identity", func() {
				newWorkloadIdentity := workloadIdentity.DeepCopy()
				Expect(workloadIdentityValidator.Validate(ctx, newWorkloadIdentity, workloadIdentity)).To(Succeed())
			})

			It("should not allow changing the tenantID or subscriptionID", func() {
				newWorkloadIdentity := workloadIdentity.DeepCopy()
				newWorkloadIdentity.Spec.TargetSystem.ProviderConfig.Raw = []byte(`
apiVersion: azure.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
clientID: "11111c4e-db61-17fa-a141-ed39b34aa561"
tenantID: "44444c4e-db61-17fa-a141-ed39b34aa561"
subscriptionID: "44444c4e-db61-17fa-a141-ed39b34aa561"
`)
				err := workloadIdentityValidator.Validate(ctx, newWorkloadIdentity, workloadIdentity)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("spec.targetSystem.providerConfig.subscriptionID: Invalid value: \"44444c4e-db61-17fa-a141-ed39b34aa561\": field is immutable, spec.targetSystem.providerConfig.tenantID: Invalid value: \"44444c4e-db61-17fa-a141-ed39b34aa561\": field is immutable"))
			})
		})

		Context("GCP Workload Identity", func() {
			BeforeEach(func() {
				workloadIdentity = &securityv1alpha1.WorkloadIdentity{
					Spec: securityv1alpha1.WorkloadIdentitySpec{
						Audiences: []string{"foo"},
						TargetSystem: securityv1alpha1.TargetSystem{
							Type: "gcp",
							ProviderConfig: &runtime.RawExtension{
								Raw: []byte(`
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
`),
							},
						},
					},
				}
			})

			It("should successfully validate the creation of a workload identity", func() {
				Expect(workloadIdentityValidator.Validate(ctx, workloadIdentity, nil)).To(Succeed())
			})

			It("should successfully validate the update of a workload identity", func() {
				newWorkloadIdentity := workloadIdentity.DeepCopy()
				newWorkloadIdentity.Spec.TargetSystem.ProviderConfig.Raw = []byte(`
apiVersion: gcp.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
projectID: "foo-valid"
credentialsConfig:
  universe_domain: "googleapis.com"
  type: "external_account"
  audience: "//iam.googleapis.com/projects/11111111/locations/global/workloadIdentityPools/foopool/providers/fooprovider"
  subject_token_type: "urn:ietf:params:oauth:token-type:jwt"
  token_url: "https://sts.googleapis.com/v1/token/new"
`)
				Expect(workloadIdentityValidator.Validate(ctx, newWorkloadIdentity, workloadIdentity)).To(Succeed())
			})

			It("should not allow changing the projectID", func() {
				newWorkloadIdentity := workloadIdentity.DeepCopy()
				newWorkloadIdentity.Spec.TargetSystem.ProviderConfig.Raw = []byte(`
apiVersion: gcp.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
projectID: "foo-valid-new"
credentialsConfig:
  universe_domain: "googleapis.com"
  type: "external_account"
  audience: "//iam.googleapis.com/projects/11111111/locations/global/workloadIdentityPools/foopool/providers/fooprovider"
  subject_token_type: "urn:ietf:params:oauth:token-type:jwt"
  token_url: "https://sts.googleapis.com/v1/token"
`)
				err := workloadIdentityValidator.Validate(ctx, newWorkloadIdentity, workloadIdentity)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("validation of target system's configuration failed: spec.targetSystem.providerConfig.projectID: Invalid value: \"foo-valid-new\": field is immutable"))
			})

			It("should not allow changing forbidden service_account_impersonation_url", func() {
				newWorkloadIdentity := workloadIdentity.DeepCopy()
				newWorkloadIdentity.Spec.TargetSystem.ProviderConfig.Raw = []byte(`
apiVersion: gcp.provider.extensions.gardener.cloud/v1alpha1
kind: WorkloadIdentityConfig
projectID: "foo-valid"
credentialsConfig:
  universe_domain: "googleapis.com"
  type: "external_account"
  audience: "//iam.googleapis.com/projects/11111111/locations/global/workloadIdentityPools/foopool/providers/fooprovider"
  subject_token_type: "urn:ietf:params:oauth:token-type:jwt"
  token_url: "https://sts.googleapis.com/v1/token"
  service_account_impersonation_url: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/foo@bar.example:generateAccessTokeninvalid"
`)
				err := workloadIdentityValidator.Validate(ctx, newWorkloadIdentity, workloadIdentity)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(Equal(`validation of target system's configuration failed: spec.targetSystem.providerConfig.credentialsConfig.service_account_impersonation_url: Invalid value: "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/foo@bar.example:generateAccessTokeninvalid": should match one of the allowed regular expressions: ^https://iamcredentials\.googleapis\.com/v1/projects/-/serviceAccounts/.+:generateAccessToken$`))
			})
		})
	})
})
