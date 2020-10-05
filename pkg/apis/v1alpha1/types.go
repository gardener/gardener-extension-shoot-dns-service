/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package v1alpha1

import (
	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const DNSStateKind = "DNSState"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DNSState describes the set of DNS entries maintained by the dns shoot service
// for a dedicated shoot cluster used to reconstruct the DNS entry objects after
// a migration.
type DNSState struct {
	metav1.TypeMeta `json:",inline"`
	Entries         []*DNSEntry `json:"entries,omitempty"`
}

type DNSEntry struct {
	Name        string                 `json:"name"`
	Labels      map[string]string      `json:"labels,omitempty"`
	Annotations map[string]string      `json:"annotations,omitempty"`
	Spec        *v1alpha1.DNSEntrySpec `json:"spec"`
}
