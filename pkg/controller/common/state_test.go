// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	v1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	api "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis"
)

var _ = Describe("StateHandler", func() {
	DescribeTable("#EnsureEntries",
		func(expectedMod bool, newEntryNames ...string) {
			handler := &StateHandler{state: &api.DNSState{}}
			handler.state.Entries = []*api.DNSEntry{
				{Name: "entry1", Spec: &v1alpha1.DNSEntrySpec{}},
				{Name: "entry2", Spec: &v1alpha1.DNSEntrySpec{}},
			}
			var newEntries []v1alpha1.DNSEntry
			for _, name := range newEntryNames {
				newEntries = append(newEntries, v1alpha1.DNSEntry{ObjectMeta: metav1.ObjectMeta{Name: name}})
			}
			mod := handler.EnsureEntries(newEntries)
			Expect(mod).To(Equal(expectedMod))
			Expect(len(handler.state.Entries)).To(Equal(len(newEntries)))
		},
		Entry("unchanged", false, "entry1", "entry2"),
		Entry("unchanged2", false, "entry2", "entry1"),
		Entry("add/remove", true, "entry1", "entry3"),
		Entry("delete one", true, "entry1"),
		Entry("replace all", true, "entry3", "entry4"),
		Entry("none", true),
	)
})
