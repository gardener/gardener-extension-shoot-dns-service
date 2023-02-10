/*
 * Copyright 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
