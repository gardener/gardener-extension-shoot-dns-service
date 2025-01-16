// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CompressedState", func() {
	It("should compress and decompress", func() {
		state := []byte(`{"entries":[{"name":"entry1","spec":{}},{"name":"entry2","spec":{}}]}`)
		data, err := CompressEntriesState(state)
		Expect(err).To(BeNil())
		Expect(data).NotTo(BeNil())

		state2, err := DecompressEntriesState(data)
		Expect(err).To(BeNil())
		Expect(state2).NotTo(BeNil())
		Expect(state).To(Equal(state2))
	})

	It("should recognise compressed state data by heuristic", func() {
		state := []byte(`{"entries":[{"name":"entry1","spec":{}},{"name":"entry2","spec":{}}]}`)
		data, err := CompressEntriesState(state)
		Expect(err).To(BeNil())
		Expect(data).NotTo(BeNil())

		Expect(LooksLikeCompressedEntriesState(data)).To(BeTrue())
		Expect(LooksLikeCompressedEntriesState(state)).To(BeFalse())
	})
})
