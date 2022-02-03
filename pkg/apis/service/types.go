// Copyright (c) 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DNSConfig configuration resource
type DNSConfig struct {
	metav1.TypeMeta

	// DNSProviderReplication contains enablement for replication of DNSProviders from shoot cluster to control plane
	DNSProviderReplication *DNSProviderReplication

	// Providers is a list of additional DNS providers that shall be enabled for this shoot cluster.
	// The primary ("external") provider at `spec.dns.provider` is added automatically
	Providers []DNSProvider

	// SyncProvidersFromShootSpecDNS is an optional flag for migrating and synchronising the providers given in the
	// shoot manifest at section `spec.dns.providers`. If true, any direct changes on the `providers` section
	// are overwritten with the content of section `spec.dns.providers`.
	SyncProvidersFromShootSpecDNS *bool
}

// DNSProviderReplication contains enablement for replication of DNSProviders from shoot cluster to control plane
type DNSProviderReplication struct {
	// Enabled if true, the replication of DNSProviders from shoot cluster to the control plane is enabled
	Enabled bool
}

// DNSProvider contains information about a DNS provider.
type DNSProvider struct {
	// Domains contains information about which domains shall be included/excluded for this provider.
	Domains *DNSIncludeExclude
	// SecretName is a name of a secret containing credentials for the stated domain and the
	// provider.
	SecretName *string
	// Type is the DNS provider type.
	Type *string
	// Zones contains information about which hosted zones shall be included/excluded for this provider.
	Zones *DNSIncludeExclude
}

// DNSIncludeExclude contains information about which domains shall be included/excluded.
type DNSIncludeExclude struct {
	// Include is a list of domains that shall be included.
	Include []string
	// Exclude is a list of domains that shall be excluded.
	Exclude []string
}
