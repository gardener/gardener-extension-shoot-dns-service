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
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	dnsapi "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"

	predutils "github.com/gardener/gardener/extensions/pkg/predicate"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
)

const (
	// Name is the name of the replication controller.
	Name = "shoot_dns_service_replication_controller"
)

// ForService returns a predicate that matches the given name of a resource.
func ForService(labelKey string) predicate.Predicate {
	return predutils.FromMapper(predutils.MapperFunc(func(e event.GenericEvent) bool {
		for k := range e.Meta.GetLabels() {
			if k == labelKey {
				return true
			}
		}
		return false
	}), predutils.CreateTrigger, predutils.UpdateNewTrigger, predutils.DeleteTrigger, predutils.GenericTrigger)
}

// AddToManager adds a DNS Service replication controller to the given Controller Manager.
func AddToManager(mgr manager.Manager) error {
	reconciler := NewReconciler(Name, config.ServiceConfig.DNSServiceConfig)
	opts := controller.Options{}
	opts.Reconciler = reconciler

	ctrl, err := controller.New(Name, mgr, opts)

	if err != nil {
		return err
	}
	predicate := ForService(reconciler.EntryLabel())
	return ctrl.Watch(&source.Kind{Type: &dnsapi.DNSEntry{}}, &handler.EnqueueRequestForObject{}, predicate)
}
