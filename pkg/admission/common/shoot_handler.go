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

package common

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ShootAdmissionHandler provides a shoot handler
type ShootAdmissionHandler struct {
	client    client.Client
	apiReader client.Reader
	decoder   runtime.Decoder
	scheme    *runtime.Scheme
}

// InjectClient injects the client.
func (s *ShootAdmissionHandler) InjectClient(c client.Client) error {
	s.client = c
	return nil
}

// InjectAPIReader injects the given apiReader into the validator.
func (s *ShootAdmissionHandler) InjectAPIReader(apiReader client.Reader) error {
	s.apiReader = apiReader
	return nil
}

// InjectScheme injects the scheme.
func (s *ShootAdmissionHandler) InjectScheme(scheme *runtime.Scheme) error {
	s.scheme = scheme
	s.decoder = serializer.NewCodecFactory(scheme).UniversalDecoder()
	return nil
}

func (s *ShootAdmissionHandler) GetDecoder() runtime.Decoder {
	return s.decoder
}

func (s *ShootAdmissionHandler) NewCodecFactory() serializer.CodecFactory {
	return serializer.NewCodecFactory(s.scheme)
}
