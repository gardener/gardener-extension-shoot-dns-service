/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SortenID", func() {
	It("should shorten ID to given max length", func() {
		id1 := "shoot--garden--something12-cf7607c1-1b8a-11e8-8c77-fa163e4902b1"
		id2 := "shoot--garden--something123-cf7607c1-1b8a-11e8-8c77-fa163e4902b1"
		id3 := "shoot--garden--something123-cf7607c1-1b8a-11e8-8c77-fa163e4902b2"
		id4 := "shoot--garden--something1234-cf7607c1-1b8a-11e8-8c77-fa163e4902b1"

		short1 := ShortenID(id1, 63)
		short2 := ShortenID(id2, 63)
		short3 := ShortenID(id3, 63)
		short4 := ShortenID(id4, 63)
		Expect(short1).To(Equal(id1))
		Expect(short2).To(Equal("shoot--garden--something123-cf7607c1-1b8a-11e8-8c7-qksvc0j2gs99"))
		Expect(len(short2)).To(Equal(63))
		Expect(short3).To(Equal("shoot--garden--something123-cf7607c1-1b8a-11e8-8c7-qksvc0j2gs9a"))
		Expect(short4).To(Equal("shoot--garden--something1234-cf7607c1-1b8a-11e8-8c-8wzf59wac3mj"))
		Expect(len(short4)).To(Equal(63))
	})
})
