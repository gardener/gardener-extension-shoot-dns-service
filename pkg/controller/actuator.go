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

package controller

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	controllerconfig "github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/imagevector"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"

	"github.com/gardener/gardener-extensions/pkg/controller"
	"github.com/gardener/gardener-extensions/pkg/controller/extension"
	"github.com/gardener/gardener-extensions/pkg/util"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/chart"
	"github.com/gardener/gardener/pkg/utils/secrets"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// ActuatorName is the name of the DNS Service actuator.
	ActuatorName = service.ServiceName + "-actuator"
	// SeedResourcesName is the name for resource describing the resources applied to the seed cluster.
	SeedResourcesName = service.ExtensionServiceName + "-seed"
	// ShootResourcesName is the name for resource describing the resources applied to the shoot cluster.
	ShootResourcesName = service.ExtensionServiceName + "-shoot"
	// KeptShootResourcesName is the name for resource describing the resources applied to the shoot cluster that should not be deleted.
	KeptShootResourcesName = service.ExtensionServiceName + "-shoot-keep"
)

// NewActuator returns an actuator responsible for Extension resources.
func NewActuator(config controllerconfig.DNSServiceConfig) extension.Actuator {
	return &actuator{
		logger:           log.Log.WithName(ActuatorName),
		controllerConfig: config,
	}
}

type actuator struct {
	applier  kubernetes.ChartApplier
	renderer chartrenderer.Interface
	client   client.Client
	config   *rest.Config

	controllerConfig controllerconfig.DNSServiceConfig

	logger logr.Logger
}

// InjectConfig injects the rest config to this actuator.
func (a *actuator) InjectConfig(config *rest.Config) error {
	a.config = config

	applier, err := kubernetes.NewChartApplierForConfig(a.config)
	if err != nil {
		return fmt.Errorf("failed to create chart applier: %v", err)
	}
	a.applier = applier

	renderer, err := chartrenderer.NewForConfig(a.config)
	if err != nil {
		return fmt.Errorf("failed to create chart renderer: %v", err)
	}
	a.renderer = renderer

	return nil
}

// InjectClient injects the controller runtime client into the reconciler.
func (a *actuator) InjectClient(client client.Client) error {
	a.client = client
	return nil
}

// Reconcile the Extension resource.
func (a *actuator) Reconcile(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	cluster, err := controller.GetCluster(ctx, a.client, ex.Namespace)
	if err != nil {
		return err
	}

	if err := a.createShootResources(ctx, cluster, ex.Namespace); err != nil {
		return err
	}
	return a.createSeedResources(ctx, cluster, ex.Namespace)
}

// Delete the Extension resource.
func (a *actuator) Delete(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	if err := a.deleteSeedResources(ctx, ex.Namespace); err != nil {
		return err
	}
	return a.deleteShootResources(ctx, ex.Namespace)
}

func (a *actuator) shootId(namespace string) string {
	return fmt.Sprintf("%s.gardener.cloud/%s", a.controllerConfig.GardenID, namespace)
}

func (a *actuator) createSeedResources(ctx context.Context, cluster *controller.Cluster, namespace string) error {
	shootKubeconfig, err := a.createKubeconfig(ctx, namespace)
	if err != nil {
		return err
	}

	chartValues := map[string]interface{}{
		"serviceName":         service.ServiceName,
		"replicas":            controller.GetReplicas(cluster, 1),
		"targetClusterSecret": shootKubeconfig.GetName(),
		"gardenId":            a.controllerConfig.GardenID,
		"shootId":             a.shootId(namespace),
		"seedId":              a.controllerConfig.SeedID,
		"dnsClass":            a.controllerConfig.DNSClass,
		"podAnnotations": map[string]interface{}{
			"checksum/secret-kubeconfig": util.ComputeChecksum(shootKubeconfig.Data),
		},
	}

	chartValues, err = chart.InjectImages(chartValues, imagevector.ImageVector(), []string{service.ImageName})
	if err != nil {
		return fmt.Errorf("failed to find image version for %s: %v", service.ImageName, err)
	}

	a.logger.Info("Component is being applied", "component", service.ExtensionServiceName, "namespace", namespace)
	return a.createManagedResource(ctx, namespace, SeedResourcesName, "seed", a.renderer, service.SeedChartName, chartValues, nil)
}

func (a *actuator) deleteSeedResources(ctx context.Context, namespace string) error {
	a.logger.Info("Component is being deleted", "component", service.ExtensionServiceName, "namespace", namespace)

	if err := controller.DeleteManagedResource(ctx, a.client, namespace, SeedResourcesName); err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := controller.WaitUntilManagedResourceDeleted(timeoutCtx, a.client, namespace, SeedResourcesName); err != nil {
		return err
	}

	secret := &corev1.Secret{}
	secret.SetName(service.SecretName)
	secret.SetNamespace(namespace)
	if err := a.client.Delete(ctx, secret); client.IgnoreNotFound(err) != nil {
		return err
	}

	shootId := a.shootId(namespace)
	list := &unstructured.UnstructuredList{}
	list.SetAPIVersion("dns.gardener.cloud/v1alpha1")
	list.SetKind("DNSEntry")
	if err := a.client.List(ctx, list, client.InNamespace(namespace), client.MatchingLabels(map[string]string{shootId: "true"})); err != nil {
		return nil
	}

	for _, item := range list.Items {
		if err := a.client.Delete(ctx, &item); client.IgnoreNotFound(err) != nil {
			return err
		}
	}
	return nil
}

func (a *actuator) createShootResources(ctx context.Context, cluster *controller.Cluster, namespace string) error {
	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1beta1")
	crd.SetKind("CustomResourceDefinition")
	if err := a.client.Get(ctx, client.ObjectKey{Name: "dnsentries.dns.gardener.cloud"}, crd); err != nil {
		return errors.Wrap(err, "could not get crd dnsentries.dns.gardener.cloud")
	}
	crd.SetResourceVersion("")
	crd.SetUID("")
	crd.SetCreationTimestamp(metav1.Time{})
	crd.SetGeneration(0)
	if err := controller.CreateManagedResourceFromUnstructured(ctx, a.client, namespace, KeptShootResourcesName, "", []*unstructured.Unstructured{crd}, true, nil); err != nil {
		return errors.Wrapf(err, "could not create managed resource %s", KeptShootResourcesName)
	}

	renderer, err := util.NewChartRendererForShoot(cluster.Shoot.Spec.Kubernetes.Version)
	if err != nil {
		return errors.Wrap(err, "could not create chart renderer")
	}

	chartValues := map[string]interface{}{
		"userName":    service.UserName,
		"serviceName": service.ServiceName,
	}
	injectedLabels := map[string]string{controller.ShootNoCleanupLabel: "true"}

	return a.createManagedResource(ctx, namespace, ShootResourcesName, "", renderer, service.ShootChartName, chartValues, injectedLabels)
}

func (a *actuator) deleteShootResources(ctx context.Context, namespace string) error {
	if err := controller.DeleteManagedResource(ctx, a.client, namespace, ShootResourcesName); err != nil {
		return err
	}
	if err := controller.DeleteManagedResource(ctx, a.client, namespace, KeptShootResourcesName); err != nil {
		return err
	}

	timeoutCtx1, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := controller.WaitUntilManagedResourceDeleted(timeoutCtx1, a.client, namespace, ShootResourcesName); err != nil {
		return err
	}

	timeoutCtx2, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return controller.WaitUntilManagedResourceDeleted(timeoutCtx2, a.client, namespace, KeptShootResourcesName)
}

func (a *actuator) createKubeconfig(ctx context.Context, namespace string) (*corev1.Secret, error) {
	certConfig := secrets.CertificateSecretConfig{
		Name:       service.SecretName,
		CommonName: service.UserName,
	}
	return util.GetOrCreateShootKubeconfig(ctx, a.client, certConfig, namespace)
}

func (a *actuator) createManagedResource(ctx context.Context, namespace, name, class string, renderer chartrenderer.Interface, chartName string, chartValues map[string]interface{}, injectedLabels map[string]string) error {
	return controller.CreateManagedResourceFromFileChart(
		ctx, a.client, namespace, name, class,
		renderer, filepath.Join(service.ChartsPath, chartName), chartName,
		chartValues, injectedLabels,
	)
}
