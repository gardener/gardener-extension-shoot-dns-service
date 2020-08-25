/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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
 *
 */

package config

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

var (
	// ServiceConfig contains configuration for the dns service.
	ServiceConfig Config
	// HealthConfig contains configuration for the health check controller
	HealthConfig HealthCheckConfig
)

// Config are options to apply when adding the dns service controller to the manager.
type Config struct {
	// ControllerOptions contains options for the controller.
	ControllerOptions controller.Options
	// DNSServiceConfig contains configuration for the lifecycle controller of the dns service.
	DNSServiceConfig DNSServiceConfig
	// IgnoreOperationAnnotation specifies whether to ignore the operation annotation or not.
	IgnoreOperationAnnotation bool
}

// DNSServiceConfig contains configuration for the dns service.
type DNSServiceConfig struct {
	SeedID   string
	DNSClass string
}

// HealthCheckConfig are options to apply when adding the health check controller to the manager.
type HealthCheckConfig struct {
	// ControllerOptions contains options for the controller.
	ControllerOptions controller.Options
	// Health contains the health config
	Health Health
}

// Health contains configuration for the health check controller
type Health struct {
	// HealthCheckSyncPeriod configured how often health checks are being executed. Defaults to '30s'
	HealthCheckSyncPeriod metav1.Duration
}
