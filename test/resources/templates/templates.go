// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package templates

const (
	// EchoserverApp is a deployment of echoheaders as Service of type LoadBalancers or as ingress with shootdns annotations
	EchoserverApp = "echoserver-app.yaml.tpl"
	// CustomDNSEntry is a DNSEntry delegated to Gardener
	CustomDNSEntry = "custom-dns-entry.yaml.tpl"
)
