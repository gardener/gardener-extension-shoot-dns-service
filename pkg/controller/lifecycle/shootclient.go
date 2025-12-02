// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"fmt"

	extensionsconfigv1alpha1 "github.com/gardener/gardener/extensions/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener/extensions/pkg/util"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type shootClientAccess interface {
	// GetShootClient returns a client for the shoot cluster in the given namespace.
	GetShootClient(ctx context.Context, namespace string) (client.Client, error)
}

type realShootClient struct {
	seedClient client.Client
}

var _ shootClientAccess = &realShootClient{}

func (c *realShootClient) GetShootClient(ctx context.Context, namespace string) (client.Client, error) {
	_, shootClient, err := util.NewClientForShoot(ctx, c.seedClient, namespace, client.Options{Scheme: c.seedClient.Scheme()}, extensionsconfigv1alpha1.RESTOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed creating client for shoot cluster: %w", err)
	}
	return shootClient, nil
}
