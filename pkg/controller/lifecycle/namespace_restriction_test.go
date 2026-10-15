// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"

	apisservice "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
)

var _ = Describe("dnsProvidersNamespaceRestrictionEnabled", func() {
	// Build an extensionContext whose useNextGenerationController() returns nextGen (via the global flag),
	// and whose DNSConfig carries the given replication settings. A nil replication arg means no
	// dnsProviderReplication block at all (the common optional case that must not panic).
	ctx := func(nextGen bool, replication *apisservice.DNSProviderReplication) extensionContext {
		return extensionContext{
			dnsconfig:    &apisservice.DNSConfig{DNSProviderReplication: replication},
			globalConfig: config.DNSServiceConfig{UseNextGenerationController: nextGen},
		}
	}
	repl := func(enabled bool, restriction *bool) *apisservice.DNSProviderReplication {
		return &apisservice.DNSProviderReplication{Enabled: enabled, NamespaceRestrictionEnabled: restriction}
	}

	DescribeTable("gating",
		func(exCtx extensionContext, expected bool) {
			a := &actuator{}
			Expect(a.dnsProvidersNamespaceRestrictionEnabled(exCtx)).To(Equal(expected))
		},
		Entry("next-gen + replication + flag → true", ctx(true, repl(true, ptr.To(true))), true),
		Entry("flag off → false", ctx(true, repl(true, ptr.To(false))), false),
		Entry("flag unset → false", ctx(true, repl(true, nil)), false),
		Entry("replication disabled → false", ctx(true, repl(false, ptr.To(true))), false),
		Entry("next-gen off → false", ctx(false, repl(true, ptr.To(true))), false),
		Entry("no replication block, next-gen on → false (no panic)", ctx(true, nil), false),
	)
})
