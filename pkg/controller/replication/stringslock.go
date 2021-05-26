/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
func (this *StringsLock) TryLock(name string) bool {
	this.lock.Lock()
	defer this.lock.Unlock()

	_, ok := this.strings[name]
	if !ok {
		this.strings[name] = struct{}{}
	}
	return !ok
}

// Unlock unlocks a name locked with `TryLock`
func (this *StringsLock) Unlock(name string) {
	this.lock.Lock()
	defer this.lock.Unlock()

	_, ok := this.strings[name]
	if !ok {
		panic("missing lock")
	}
	delete(this.strings, name)
}
