// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	"k8s.io/apimachinery/pkg/types"
)

// DNSService contains configuration for the lifecycle controller of the dns service.
var DNSService DNSServiceConfig

// DNSServiceConfig contains configuration for the dns service.
type DNSServiceConfig struct {
	SeedID                            string
	DNSClass                          string
	RemoteDefaultDomainSecret         *types.NamespacedName
	ManageDNSProviders                bool
	ReplicateDNSProviders             bool
	InternalGCPWorkloadIdentityConfig config.InternalGCPWorkloadIdentityConfig
}
