// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
