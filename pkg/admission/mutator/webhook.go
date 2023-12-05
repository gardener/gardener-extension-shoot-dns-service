// Copyright (c) 2022 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package mutator

import (
	extensionswebhook "github.com/gardener/gardener/extensions/pkg/webhook"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

const (
	MutatorName = "mutator"
	MutatorPath = "/webhooks/mutate"
)

var logger = log.Log.WithName("shoot-dns-service-mutator-webhook")

// New creates a new webhook that validates Shoot resources.
func New(mgr manager.Manager) (*extensionswebhook.Webhook, error) {
	logger.Info("Setting up webhook", "name", MutatorName)

	return extensionswebhook.New(mgr, extensionswebhook.Args{
		Provider: "shoot-dns-service",
		Name:     MutatorName,
		Path:     MutatorPath,
		Mutators: map[extensionswebhook.Mutator][]extensionswebhook.Type{
			NewShootMutator(mgr): {{Obj: &gardencorev1beta1.Shoot{}}},
		},
		Target: extensionswebhook.TargetSeed,
		ObjectSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{"extensions.extensions.gardener.cloud/shoot-dns-service": "true"},
		},
	})
}
