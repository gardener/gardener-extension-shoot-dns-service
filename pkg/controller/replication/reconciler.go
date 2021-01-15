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

package replication

import (
	"context"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/common"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"

	dnsapi "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	extapi "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type reconciler struct {
	*common.Env
}

// newReconciler creates a new reconcile.Reconciler that reconciles
// Extension resources of Gardener's `extensions.gardener.cloud` API group.
func newReconciler(name string, controllerConfig config.DNSServiceConfig) *reconciler {
	return &reconciler{
		Env: common.NewEnv(name, controllerConfig),
	}
}

////////////////////////////////////////////////////////////////////////////////
// entry reconcilation

func (r *reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	result := reconcile.Result{}

	ext, err := r.findExtension(ctx, req.Namespace)
	if err != nil {
		return result, err
	}
	if common.IsMigrating(ext) {
		return result, nil
	}
	statehandler, err := common.NewStateHandler(ctx, r.Env, ext, false)
	if err != nil {
		return result, err
	}

	mod := false
	entry := &dnsapi.DNSEntry{}
	err = r.Client().Get(ctx, req.NamespacedName, entry)
	if err != nil {
		if !errors.IsNotFound(err) {
			return result, err
		}
		mod = r.delete(statehandler, req) || mod
	} else {
		if entry.DeletionTimestamp != nil {
			mod = r.delete(statehandler, req) || mod
		} else {
			mod = r.reconcile(statehandler, entry) || mod
		}
	}
	if mod {
		return result, statehandler.Update()
	}
	return result, nil
}

func (r *reconciler) reconcile(statehandler *common.StateHandler, entry *dnsapi.DNSEntry) bool {
	return statehandler.EnsureEntryFor(entry)
}

func (r *reconciler) delete(statehandler *common.StateHandler, req reconcile.Request) bool {
	return statehandler.EnsureEntryDeleted(req.Name)
}

////////////////////////////////////////////////////////////////////////////////
// extension handling

func (r *reconciler) findExtension(ctx context.Context, namespace string) (*extapi.Extension, error) {
	return common.FindExtension(ctx, r.Client(), namespace)
}
