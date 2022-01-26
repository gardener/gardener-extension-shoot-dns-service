// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package lifecycle

import (
	"time"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"

	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	// Name is the name of the lifecycle controller.
	Name = "shoot_dns_service_lifecycle_controller"
	// FinalizerSuffix is the finalizer suffix for the DNS Service controller.
	FinalizerSuffix = service.ExtensionServiceName
)

// DefaultAddOptions contains configuration for the dns service.
var DefaultAddOptions = AddOptions{}

// AddOptions are options to apply when adding the dns service controller to the manager.
type AddOptions struct {
	// Controller contains options for the controller.
	Controller controller.Options
	// IgnoreOperationAnnotation specifies whether to ignore the operation annotation or not.
	IgnoreOperationAnnotation bool
	// UseTokenRequestor specifies whether the token requestor shall be used for the dns-controller.
	UseTokenRequestor bool
	// UseProjectedTokenMount specifies whether the projected token mount shall be used for the
	// dns-controller.
	UseProjectedTokenMount bool
}

// AddToManager adds a controller with the default Options to the given Controller Manager.
func AddToManager(mgr manager.Manager) error {
	return AddToManagerWithOptions(mgr, DefaultAddOptions)
}

// AddToManagerWithOptions adds a DNS Service Lifecycle controller to the given Controller Manager.
func AddToManagerWithOptions(mgr manager.Manager, opts AddOptions) error {
	return extension.Add(mgr, extension.AddArgs{
		Actuator:          NewActuator(config.DNSService, opts.UseTokenRequestor, opts.UseProjectedTokenMount),
		ControllerOptions: opts.Controller,
		Name:              Name,
		FinalizerSuffix:   FinalizerSuffix,
		Resync:            60 * time.Minute,
		Predicates:        extension.DefaultPredicates(opts.IgnoreOperationAnnotation),
		Type:              service.ExtensionType,
	})
}
