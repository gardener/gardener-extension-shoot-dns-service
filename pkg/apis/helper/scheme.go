// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package helper

import (
	"fmt"

	extapi "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	api "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/install"
)

var (
	// Scheme is a scheme with the types relevant for vSphere actuators.
	Scheme *runtime.Scheme

	decoder runtime.Decoder
)

func init() {
	Scheme = runtime.NewScheme()
	utilruntime.Must(install.AddToScheme(Scheme))

	decoder = serializer.NewCodecFactory(Scheme).UniversalDecoder()
}

func GetExtensionState(ext *extapi.Extension) (*api.DNSState, error) {
	state := &api.DNSState{}
	if ext.Status.State != nil && ext.Status.State.Raw != nil {
		if _, _, err := decoder.Decode(ext.Status.State.Raw, nil, state); err != nil {
			return state, fmt.Errorf("could not decode extension state: %w", err)
		}
	}
	return state, nil
}
