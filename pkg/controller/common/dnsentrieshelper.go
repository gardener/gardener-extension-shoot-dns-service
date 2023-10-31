/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package common

import (
	"context"
	"fmt"

	dnsapi "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

////////////////////////////////////////////////////////////////////////////////
// shoot DNS entries helper

// ShootDNSEntryLabelKey is the label key for DNS entries managed for shoots
const ShootDNSEntryLabelKey = "gardener.cloud/shoot-id"

type ShootDNSEntriesHelper struct {
	ctx     context.Context
	client  client.Client
	ext     *v1alpha1.Extension
	cluster *controller.Cluster
}

func NewShootDNSEntriesHelper(ctx context.Context, client client.Client, ext *v1alpha1.Extension) *ShootDNSEntriesHelper {
	return &ShootDNSEntriesHelper{
		ctx:    ctx,
		client: client,
		ext:    ext,
	}
}

func (h *ShootDNSEntriesHelper) Context() context.Context {
	return h.ctx
}

func (h *ShootDNSEntriesHelper) Extension() *v1alpha1.Extension {
	return h.ext
}

func (h *ShootDNSEntriesHelper) GetCluster() (*controller.Cluster, error) {
	if h.cluster != nil {
		return h.cluster, nil
	}
	cluster, err := controller.GetCluster(h.ctx, h.client, h.ext.Namespace)
	if err != nil {
		return nil, err
	}
	h.cluster = cluster
	return h.cluster, nil
}

func (h *ShootDNSEntriesHelper) ShootID() (string, string, error) {
	cluster, err := h.GetCluster()
	if err != nil {
		return "", "", err
	}
	if cluster.Shoot.Status.ClusterIdentity == nil {
		return "", "", fmt.Errorf("missing shoot cluster identity")
	}
	return *cluster.Shoot.Status.ClusterIdentity, ShortenID(*cluster.Shoot.Status.ClusterIdentity, 63), nil
}

func (h *ShootDNSEntriesHelper) ShootDNSEntryMatchingLabel() (client.MatchingLabels, error) {
	_, labelValue, err := h.ShootID()
	if err != nil {
		return nil, err
	}
	return client.MatchingLabels{ShootDNSEntryLabelKey: labelValue}, nil
}

func (h *ShootDNSEntriesHelper) List() ([]dnsapi.DNSEntry, error) {
	matchingLabel, err := h.ShootDNSEntryMatchingLabel()
	if err != nil {
		return nil, err
	}
	list := &dnsapi.DNSEntryList{}
	err = h.client.List(h.ctx, list, client.InNamespace(h.ext.Namespace), matchingLabel)
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	return list.Items, nil
}

func (h *ShootDNSEntriesHelper) DeleteAll() error {
	matchingLabel, err := h.ShootDNSEntryMatchingLabel()
	if err != nil {
		return err
	}
	return h.client.DeleteAllOf(h.ctx, &dnsapi.DNSEntry{}, client.InNamespace(h.ext.Namespace), matchingLabel)
}

// ForceDeleteAll forces deletion of DNSEntries by removing the finalizers first.
// Warning: calling this method can result in leaked DNS record sets in the infrastructure and should only be used as last resort.
func (h *ShootDNSEntriesHelper) ForceDeleteAll() error {
	err := h.DeleteAll()
	if err != nil {
		return err
	}

	entries, err := h.List()
	if err != nil {
		return err
	}

	for _, entry := range entries {
		patch := client.MergeFrom(entry.DeepCopy())
		entry.SetFinalizers(nil)
		if err := client.IgnoreNotFound(h.client.Patch(h.ctx, &entry, patch)); err != nil {
			return fmt.Errorf("removing finalizers for DNSEntry %s failed: %w", entry.Name, err)
		}
	}

	return nil
}
