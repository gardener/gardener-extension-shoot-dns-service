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

package replication

import (
	"fmt"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("StringsLock", func() {
	It("should trylock and unlock correctly", func() {
		lock := NewStringsLock()
		Expect(lock.TryLock("foo")).To(BeTrue())
		Expect(lock.TryLock("bar")).To(BeTrue())
		Expect(lock.TryLock("foo")).To(BeFalse())
		lock.Unlock("foo")
		lock.Unlock("bar")
		Expect(lock.TryLock("foo")).To(BeTrue())
		Expect(lock.TryLock("foo")).To(BeFalse())
		lock.Unlock("foo")
		Expect(len(lock.strings)).To(Equal(0))
	})

	It("should trylock and unlock concurrently without deadlock", func() {
		f := func(lock *StringsLock, idx int) {
			name := fmt.Sprintf("name%d", idx)
			for j := 0; j < 1000; j++ {
				for !lock.TryLock(name) {
					time.Sleep(1 * time.Microsecond)
				}
				time.Sleep(5 * time.Microsecond)
				lock.Unlock(name)
			}
		}
		testConcurrently(20, f)
	})
})

func testConcurrently(threadCount int, f func(lock *StringsLock, idx int)) {
	lock := NewStringsLock()
	wg := sync.WaitGroup{}
	wg.Add(threadCount)
	for i := 0; i < threadCount; i++ {
		go func(k int) {
			f(lock, k%3)
			wg.Done()
		}(i)
	}
	wg.Wait()
	Expect(len(lock.strings)).To(Equal(0))
}
