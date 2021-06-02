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
	"fmt"
	"time"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/common"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"k8s.io/apimachinery/pkg/util/wait"

	dnsapi "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	extapi "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type reconciler struct {
	*common.Env
	lock *StringsLock
}

// newReconciler creates a new reconcile.Reconciler that reconciles
// Extension resources of Gardener's `extensions.gardener.cloud` API group.
func newReconciler(name string, controllerConfig config.DNSServiceConfig) *reconciler {
	return &reconciler{
		Env:  common.NewEnv(name, controllerConfig),
		lock: NewStringsLock(),
	}
}

////////////////////////////////////////////////////////////////////////////////
// entry reconcilation

func (r *reconciler) Reconcile(ctx context.Context, req reconcile.Request) (reconcile.Result, error) {
	result := reconcile.Result{}

	// ensure that only one DNSEntry is reconciled per extension (shoot) to avoid parallel conflicting updates
	if !r.lock.TryLock(req.Namespace) {
		r.Env.Info("delaying as namespace already locked", "namespace", req.Namespace, "entry", req.Name)
		result.Requeue = true
		result.RequeueAfter = wait.Jitter(1*time.Second, 0)
		return result, nil
	}
	defer r.lock.Unlock(req.Namespace)

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

	entry := &dnsapi.DNSEntry{}
	err = r.Client().Get(ctx, req.NamespacedName, entry)
	var format string
	if err != nil {
		if !errors.IsNotFound(err) {
			return result, err
		}
		r.delete(statehandler, req)
		format = "entry %s deleted"
	} else {
		if entry.DeletionTimestamp != nil {
			r.delete(statehandler, req)
			format = "entry %s deleting"
		} else {
			r.reconcile(statehandler, entry)
			format = "entry %s created or updated"
		}
	}
	reason := fmt.Sprintf(format, req.Name)
	return result, statehandler.Update(reason)
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
	// apiReader is used as copy from cache is sometimes outdated
	return common.FindExtension(ctx, r.APIReader(), namespace)
}
