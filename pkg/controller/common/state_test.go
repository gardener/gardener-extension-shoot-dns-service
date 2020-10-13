/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package common

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
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
