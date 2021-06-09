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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DNSConfig configuration resource
type DNSConfig struct {
	metav1.TypeMeta `json:",inline"`

	// DNSProviderReplication contains enablement for replication of DNSProviders from shoot cluster to control plane
	// +optional
	DNSProviderReplication *DNSProviderReplication `json:"dnsProviderReplication,omitempty"`
}

// DNSProviderReplication contains enablement for replication of DNSProviders from shoot cluster to control plane
type DNSProviderReplication struct {
	// Enabled if true, the replication of DNSProviders from shoot cluster to the control plane is enabled
	Enabled bool `json:"enabled"`
}
