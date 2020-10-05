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

/**
	Overview
		- Tests the health checks for the shoot-dns-service extension.

	Prerequisites
		- A Shoot exists.

	Test-case:
		1) Extension CRD
			1.1) HealthCondition Type: ShootControlPlaneHealthy
				-  update the ManagedResource 'extension-shoot-dns-service-seed' and verify the health check conditions in the Extension CRD status.
			1.2) HealthCondition Type: ShootSystemComponentsHealthy
				-  update the ManagedResource 'extension-shoot-dns-service-shoot' and verify the health check conditions in the Extension CRD status.

 **/

package healthcheck

import (
	"context"
	"fmt"
	"time"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/lifecycle"

	healthcheckoperation "github.com/gardener/gardener/extensions/test/integration/healthcheck"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	"github.com/gardener/gardener/test/framework"
	"github.com/onsi/ginkgo"
)

const (
	timeout = 5 * time.Minute
)

var _ = ginkgo.Describe("Extension-shoot-dns-service integration test: health checks", func() {
	f := framework.NewShootFramework(nil)

	ginkgo.Context("Extension", func() {
		ginkgo.Context("Condition type: ShootControlPlaneHealthy", func() {
			f.Serial().Release().CIt(fmt.Sprintf("Extension CRD should contain unhealthy condition due to ManagedResource '%s' is unhealthy", lifecycle.SeedResourcesName), func(ctx context.Context) {
				err := healthcheckoperation.ExtensionHealthCheckWithManagedResource(ctx, timeout, f, "shoot-dns-service", lifecycle.SeedResourcesName, gardencorev1beta1.ShootControlPlaneHealthy)
				framework.ExpectNoError(err)
			}, timeout)
		})

		ginkgo.Context("Condition type: ShootSystemComponentsHealthy", func() {
			f.Serial().Release().CIt(fmt.Sprintf("Extension CRD should contain unhealthy condition due to ManagedResource '%s' is unhealthy", lifecycle.ShootResourcesName), func(ctx context.Context) {
				err := healthcheckoperation.ExtensionHealthCheckWithManagedResource(ctx, timeout, f, "shoot-dns-service", lifecycle.ShootResourcesName, gardencorev1beta1.ShootSystemComponentsHealthy)
				framework.ExpectNoError(err)
			}, timeout)
		})
	})
})
