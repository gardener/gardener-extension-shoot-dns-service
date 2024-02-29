// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package system_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestShootDNS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ShootDNS Test Suite")
}
