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

package common

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
)

type Env struct {
	name       string
	restConfig *rest.Config
	client     client.Client
	config     config.DNSServiceConfig
	apiReader  client.Reader
	logr.Logger
}

func NewEnv(name string, config config.DNSServiceConfig) *Env {
	return &Env{
		name:   name,
		config: config,
		Logger: log.Log.WithName(name),
	}
}

func (e *Env) RestConfig() *rest.Config {
	return e.restConfig
}

func (e *Env) Client() client.Client {
	return e.client
}

func (e *Env) Config() *config.DNSServiceConfig {
	return &e.config
}

func (e *Env) APIReader() client.Reader {
	return e.apiReader
}

func (e *Env) CreateObject(ctx context.Context, obj client.Object, opts ...client.CreateOption) error {
	return e.client.Create(ctx, obj, opts...)
}

func (e *Env) GetObject(ctx context.Context, key client.ObjectKey, obj client.Object) error {
	return e.client.Get(ctx, key, obj)
}

func (e *Env) UpdateObject(ctx context.Context, obj client.Object, opts ...client.UpdateOption) error {
	return e.client.Update(ctx, obj, opts...)
}

// InjectFunc enables dependency injection into the actuator.
func (e *Env) InjectFunc(f inject.Func) error {
	return nil
}

// InjectLogger injects the controller runtime client into the reconciler.
func (e *Env) InjectLogger(l logr.Logger) error {
	e.Logger = l.WithName(e.name)
	return nil
}

// InjectConfig injects the rest configuration into the reconciler.
func (e *Env) InjectConfig(config *rest.Config) error {
	e.restConfig = config
	return nil
}

// InjectAPIReader injects the APIReader into the reconciler.
func (e *Env) InjectAPIReader(reader client.Reader) error {
	e.apiReader = reader
	return nil
}

// InjectConfig injects the caching client into the reconciler.
func (e *Env) InjectClient(client client.Client) error {
	e.client = client
	return nil
}
