// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package replication

import (
	"sync"
)

// StringsLock provides separate lock per given name with TryLock/Unlock semantics.
type StringsLock struct {
	lock    sync.Mutex
	strings map[string]struct{}
}

// NewStringsLock creates a new StringsLock object
func NewStringsLock() *StringsLock {
	return &StringsLock{strings: map[string]struct{}{}}
}

// TryLock returns true if the name is successfully locked, i.e. not locked by another consumer.
// If true is returned, it must be unlocked with `Unlock`
func (l *StringsLock) TryLock(name string) bool {
	l.lock.Lock()
	defer l.lock.Unlock()

	_, ok := l.strings[name]
	if !ok {
		l.strings[name] = struct{}{}
	}
	return !ok
}

// Unlock unlocks a name locked with `TryLock`
func (l *StringsLock) Unlock(name string) {
	l.lock.Lock()
	defer l.lock.Unlock()

	_, ok := l.strings[name]
	if !ok {
		panic("missing lock")
	}
	delete(l.strings, name)
}
