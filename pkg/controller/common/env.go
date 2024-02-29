// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager"

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

func NewEnv(name string, mgr manager.Manager, config config.DNSServiceConfig) *Env {
	return &Env{
		name:       name,
		restConfig: mgr.GetConfig(),
		client:     mgr.GetClient(),
		apiReader:  mgr.GetAPIReader(),
		config:     config,
		Logger:     log.Log.WithName(name),
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
