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
	"fmt"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/runtime/inject"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

type Env struct {
	name   string
	client client.Client
	ctx    context.Context
	config config.DNSServiceConfig
	logr.Logger
}

func NewEnv(name string, config config.DNSServiceConfig) *Env {
	return &Env{
		name:   name,
		ctx:    context.Background(),
		config: config,
		Logger: log.Log.WithName(name),
	}
}

func (e *Env) Infof(msg string, args ...interface{}) {
	e.Info(fmt.Sprintf(msg, args...), "component", service.ServiceName)
}

func (e *Env) Context() context.Context {
	return e.ctx
}

func (e *Env) Client() client.Client {
	return e.client
}

func (e *Env) Config() *config.DNSServiceConfig {
	return &e.config
}

func (e *Env) CreateObject(obj runtime.Object, opts ...client.CreateOption) error {
	return e.client.Create(e.ctx, obj, opts...)
}

func (e *Env) GetObject(key client.ObjectKey, obj runtime.Object) error {
	return e.client.Get(e.ctx, key, obj)
}

func (e *Env) UpdateObject(obj runtime.Object, opts ...client.UpdateOption) error {
	return e.client.Update(e.ctx, obj, opts...)
}

func (e *Env) ListObjects(list runtime.Object, opts ...client.ListOption) error {
	return e.client.List(e.ctx, list, opts...)
}

// EntryLabel returns the label key for DNS entries managed for shoots
func (e *Env) EntryLabel() string {
	return "gardener.cloud/shoot-id"
}

// InjectFunc enables dependency injection into the actuator.
func (e *Env) InjectFunc(f inject.Func) error {
	return nil
}

// InjectClient injects the controller runtime client into the reconciler.
func (e *Env) InjectClient(client client.Client) error {
	e.client = client
	return nil
}

// InjectLogger injects the controller runtime client into the reconciler.
func (e *Env) InjectLogger(l logr.Logger) error {
	e.Logger = l.WithName(e.name)
	return nil
}
