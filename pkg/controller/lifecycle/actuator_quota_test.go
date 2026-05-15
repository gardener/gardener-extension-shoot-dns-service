// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
)

var _ = Describe("GetDefaultDomainQuota", func() {
	DescribeTable("should return correct quota",
		func(defaultQuota, maxQuota int32, annotation *string, expectedQuota int32, expectError bool, errorSubstring string) {
			// Set up config
			cfg := config.DNSServiceConfig{
				DefaultExternalProviderEntriesQuota:    defaultQuota,
				DefaultExternalProviderEntriesQuotaMax: maxQuota,
			}

			// Create cluster with optional annotation
			cluster := &extensionscontroller.Cluster{
				Shoot: &gardencorev1beta1.Shoot{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-shoot",
						Namespace: "garden-test",
					},
				},
			}

			if annotation != nil {
				cluster.Shoot.Annotations = map[string]string{
					ShootDNSServiceDefaultExternalProviderEntriesQuotaAnnotation: *annotation,
				}
			}

			// Call function
			quota, err := GetDefaultDomainQuota(cfg, cluster)

			// Verify results
			if expectError {
				Expect(err).To(HaveOccurred())
				if errorSubstring != "" {
					Expect(err.Error()).To(ContainSubstring(errorSubstring))
				}
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(quota).To(Equal(expectedQuota))
			}
		},
		Entry("quotas disabled - no annotation", int32(0), int32(0), nil, int32(0), false, ""),
		Entry("quotas disabled - with annotation", int32(0), int32(0), new("50"), int32(0), false, ""),
		Entry("default quota - no annotation", int32(100), int32(0), nil, int32(100), false, ""),
		Entry("default quota - with valid annotation within default", int32(100), int32(0), new("80"), int32(80), false, ""),
		Entry("default quota - with valid annotation exceeding default without max", int32(100), int32(0), new("150"), int32(0), true, "exceeds maximum allowed quota 100"),
		Entry("default quota with max - valid annotation within max", int32(100), int32(200), new("150"), int32(150), false, ""),
		Entry("default quota with max - annotation equals max", int32(100), int32(200), new("200"), int32(200), false, ""),
		Entry("default quota with max - annotation exceeds max", int32(100), int32(200), new("250"), int32(0), true, "exceeds maximum allowed quota 200"),
		Entry("annotation with invalid format", int32(100), int32(0), new("invalid"), int32(0), true, "failed to parse"),
		Entry("annotation with negative value", int32(100), int32(0), new("-10"), int32(0), true, "invalid default external provider entries quota"),
		Entry("annotation with zero value", int32(100), int32(0), new("0"), int32(0), true, "invalid default external provider entries quota"),
		Entry("annotation with empty string - returns default", int32(100), int32(0), new(""), int32(100), false, ""),
		Entry("small default quota - annotation within limit", int32(10), int32(0), new("5"), int32(5), false, ""),
		Entry("small default quota - annotation exceeds limit", int32(10), int32(0), new("15"), int32(0), true, "exceeds maximum allowed quota 10"),
	)
})
