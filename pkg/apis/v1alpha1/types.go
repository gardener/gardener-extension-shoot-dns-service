// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
