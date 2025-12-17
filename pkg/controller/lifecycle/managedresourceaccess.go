// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"path/filepath"

	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/utils/managedresources"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-dns-service/charts"
)

type managedResourcesAccess interface {
	// CreateOrUpdate creates or updates a managed resource.
	CreateOrUpdate(ctx context.Context, namespace, name, class string, renderer chartrenderer.Interface, chartName string, chartValues map[string]any, injectedLabels map[string]string) error
	// Delete deletes the managed resource and its secrets with the given name in the given namespace.
	Delete(ctx context.Context, namespace string, name string) error
	// WaitUntilDeleted waits until the given managed resource is deleted.
	WaitUntilDeleted(ctx context.Context, namespace, name string) error
	// SetKeepObjects updates the keepObjects field of the managed resource with the given name in the given namespace.
	SetKeepObjects(ctx context.Context, namespace, name string, keepObjects bool) error
}

type realManagedResourcesAccess struct {
	client client.Client
}

var _ managedResourcesAccess = &realManagedResourcesAccess{}

func (a *realManagedResourcesAccess) CreateOrUpdate(ctx context.Context, namespace, name, class string, renderer chartrenderer.Interface, chartName string, chartValues map[string]any, injectedLabels map[string]string) error {
	chartPath := filepath.Join(charts.ChartsPath, chartName)
	chart, err := renderer.RenderEmbeddedFS(charts.Internal, chartPath, chartName, namespace, chartValues)
	if err != nil {
		return err
	}

	data := map[string][]byte{chartName: chart.Manifest()}
	keepObjects := false
	forceOverwriteAnnotations := false
	return managedresources.Create(ctx, a.client, namespace, name, nil, false, class, data, &keepObjects, injectedLabels, &forceOverwriteAnnotations)
}

func (a *realManagedResourcesAccess) Delete(ctx context.Context, namespace string, name string) error {
	return managedresources.Delete(ctx, a.client, namespace, name, false)
}

func (a *realManagedResourcesAccess) WaitUntilDeleted(ctx context.Context, namespace, name string) error {
	return managedresources.WaitUntilDeleted(ctx, a.client, namespace, name)
}

func (a *realManagedResourcesAccess) SetKeepObjects(ctx context.Context, namespace, name string, keepObjects bool) error {
	return managedresources.SetKeepObjects(ctx, a.client, namespace, name, keepObjects)
}
