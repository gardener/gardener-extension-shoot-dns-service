// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"

	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetSecretRefFromDNSRecordExternal reads the secret reference and type from the DNSRecord external
// If the DNSRecord resource is not found, it returns nil.
func GetSecretRefFromDNSRecordExternal(ctx context.Context, c client.Client, namespace, shootName string) (*corev1.SecretReference, string, *string, error) {
	dns := &extensionsv1alpha1.DNSRecord{}
	if err := c.Get(ctx, kutil.Key(namespace, shootName+"-external"), dns); client.IgnoreNotFound(err) != nil && !meta.IsNoMatchError(err) {
		return nil, "", nil, err
	}

	return &dns.Spec.SecretRef, dns.Spec.Type, dns.Spec.Zone, nil
}
