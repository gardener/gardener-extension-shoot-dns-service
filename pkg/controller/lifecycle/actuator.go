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
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/common"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/imagevector"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	resourceapi "github.com/gardener/gardener-resource-manager/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/extensions/pkg/controller"
	controllererror "github.com/gardener/gardener/extensions/pkg/controller/error"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	"github.com/gardener/gardener/extensions/pkg/util"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils"
	"github.com/gardener/gardener/pkg/utils/chart"
	"github.com/gardener/gardener/pkg/utils/managedresources"
	"github.com/gardener/gardener/pkg/utils/secrets"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	// OwnerName is the name of the DNSOwner object created for the shoot dns service
	OwnerName = service.ServiceName
)

// NewActuator returns an actuator responsible for Extension resources.
func NewActuator(config config.DNSServiceConfig) extension.Actuator {
	return &actuator{
		Env: common.NewEnv(ActuatorName, config),
	}
}

type actuator struct {
	*common.Env
	applier  kubernetes.ChartApplier
	renderer chartrenderer.Interface
}

// InjectConfig injects the rest config to this actuator.
func (a *actuator) InjectConfig(config *rest.Config) error {
	err := a.Env.InjectConfig(config)
	if err != nil {
		return err
	}

	applier, err := kubernetes.NewChartApplierForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create chart applier: %v", err)
	}
	a.applier = applier

	renderer, err := chartrenderer.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create chart renderer: %v", err)
	}
	a.renderer = renderer

	return nil
}

// Reconcile the Extension resource.
func (a *actuator) Reconcile(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	cluster, err := controller.GetCluster(ctx, a.Client(), ex.Namespace)
	if err != nil {
		return err
	}

	resurrection := false
	if ex.Status.State != nil && !common.IsMigrating(ex) {
		resurrection, err = a.ResurrectFrom(ctx, ex)
		if err != nil {
			return err
		}
	}

	// Shoots that don't specify a DNS domain or that are scheduled to a seed that is tainted with "DNS disabled"
	// don't get an DNS service

	if !seedSettingShootDNSEnabled(cluster.Seed.Spec.Settings) ||
		cluster.Shoot.Spec.DNS == nil {
		a.Info("DNS domain is not specified, the seed .spec.settings.shootDNS.enabled=false, therefore no shoot dns service is installed", "shoot", ex.Namespace)
		return a.Delete(ctx, ex)
	}

	if err := a.createOrUpdateShootResources(ctx, cluster, ex.Namespace); err != nil {
		return err
	}
	return a.createOrUpdateSeedResources(ctx, cluster, ex, !resurrection, true)
}

func (a *actuator) ResurrectFrom(ctx context.Context, ex *extensionsv1alpha1.Extension) (bool, error) {
	owner := &v1alpha1.DNSOwner{}

	err := a.GetObject(ctx, client.ObjectKey{Name: a.OwnerName(ex.Namespace)}, owner)
	if err == nil || !k8serr.IsNotFound(err) {
		return false, err
	}
	// Ok, Owner object lost. This might have several reasons, we have to try to
	// exclude a human error before initiating a resurrection

	handler, err := common.NewStateHandler(ctx, a.Env, ex, false)
	if err != nil {
		return false, err
	}
	handler.Infof("owner object not found")
	err = a.GetObject(ctx, client.ObjectKey{Namespace: ex.Namespace, Name: SeedResourcesName}, &resourceapi.ManagedResource{})
	if err == nil || !k8serr.IsNotFound(err) {
		// a potentially missing DNSOwner object will be reconciled by resource manager
		return false, err
	}

	handler.Infof("resources object not found, also -> trying to resurrect DNS entries before setting up new owner")

	found, err := handler.ShootDNSEntriesHelper().List()
	if err != nil {
		return true, err
	}
	names := sets.String{}
	for _, item := range found {
		names.Insert(item.Name)
	}
	var lasterr error
	for _, item := range handler.StateItems() {
		if names.Has(item.Name) {
			continue
		}
		obj := &v1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:        item.Name,
				Namespace:   ex.Namespace,
				Labels:      item.Labels,
				Annotations: item.Annotations,
			},
			Spec: *item.Spec,
		}
		err := a.CreateObject(ctx, obj)
		if err != nil && !k8serr.IsAlreadyExists(err) {
			lasterr = err
		}
	}

	// the new onwer will be reconciled by resource manger after re-/creating
	// the seed resource object later on
	return true, lasterr
}

// Delete the Extension resource.
func (a *actuator) Delete(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	return a.delete(ctx, ex, false)
}

func (a *actuator) delete(ctx context.Context, ex *extensionsv1alpha1.Extension, migrate bool) error {
	if err := a.deleteSeedResources(ctx, ex, migrate); err != nil {
		return err
	}
	return a.deleteShootResources(ctx, ex.Namespace)
}

// Restore the Extension resource.
func (a *actuator) Restore(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	return a.Reconcile(ctx, ex)
}

// Migrate the Extension resource.
func (a *actuator) Migrate(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	// Keep objects for shoot managed resources so that they are not deleted from the shoot during the migration
	if err := managedresources.SetKeepObjects(ctx, a.Client(), ex.GetNamespace(), ShootResourcesName, true); err != nil {
		return err
	}

	return a.delete(ctx, ex, true)
}

func (a *actuator) createOrUpdateSeedResources(ctx context.Context, cluster *controller.Cluster, ex *extensionsv1alpha1.Extension,
	refresh bool, deploymentEnabled bool) error {
	namespace := ex.Namespace
	shootKubeconfig, err := a.createKubeconfig(ctx, namespace)
	if err != nil {
		return err
	}

	handler, err := common.NewStateHandler(ctx, a.Env, ex, refresh)
	if err != nil {
		return err
	}
	err = handler.Update()
	if err != nil {
		return err
	}

	shootID, creatorLabelValue, err := handler.ShootDNSEntriesHelper().ShootID()
	if err != nil {
		return err
	}

	seedID := a.Config().SeedID
	if seedID == "" {
		// load seed cluster-identity from kube-system namespace
		cm := &corev1.ConfigMap{}
		err = a.Client().Get(context.TODO(), client.ObjectKey{Namespace: "kube-system", Name: "cluster-identity"}, cm)
		if err != nil {
			return errors.Wrap(err, "cannot get seed identity from configmap 'kube-system/cluster-identity'")
		}
		var ok bool
		seedID, ok = cm.Data["cluster-identity"]
		if !ok {
			return fmt.Errorf("'cluster-identity' not found in configmap 'kube-system/cluster-identity'")
		}
		a.Config().SeedID = seedID
	}

	replicas := 1
	if !deploymentEnabled {
		replicas = 0
	}
	chartValues := map[string]interface{}{
		"serviceName":         service.ServiceName,
		"replicas":            controller.GetReplicas(cluster, replicas),
		"targetClusterSecret": shootKubeconfig.GetName(),
		"creatorLabelValue":   creatorLabelValue,
		"shootId":             shootID,
		"shootMigrationId":    a.oldShootId(namespace),
		"seedId":              seedID,
		"dnsClass":            a.Config().DNSClass,
		"dnsOwner":            a.OwnerName(namespace),
		"shootActive":         !common.IsMigrating(ex),
		"podAnnotations": map[string]interface{}{
			"checksum/secret-kubeconfig": utils.ComputeChecksum(shootKubeconfig.Data),
		},
	}

	chartValues, err = chart.InjectImages(chartValues, imagevector.ImageVector(), []string{service.ImageName})
	if err != nil {
		return fmt.Errorf("failed to find image version for %s: %v", service.ImageName, err)
	}

	a.Info("Component is being applied", "component", service.ExtensionServiceName, "namespace", namespace)
	return a.createOrUpdateManagedResource(ctx, namespace, SeedResourcesName, "seed", a.renderer, service.SeedChartName, chartValues, nil)
}

func (a *actuator) deleteSeedResources(ctx context.Context, ex *extensionsv1alpha1.Extension, migrate bool) error {
	namespace := ex.Namespace
	a.Info("Component is being deleted", "component", service.ExtensionServiceName, "namespace", namespace)

	if !migrate {
		entriesHelper := common.NewShootDNSEntriesHelper(ctx, a.Client(), ex)
		list, err := entriesHelper.List()
		if err != nil {
			return err
		}
		if len(list) > 0 {
			// need to wait until all shoot DNS entries have been deleted
			// for robustness scale deployment of shoot-dns-service-seed down to 0
			// and delete all shoot DNS entries
			err := a.cleanupShootDNSEntries(entriesHelper)
			if err != nil {
				return errors.Wrap(err, "cleanupShootDNSEntries failed")
			}
			a.Info("Waiting until all shoot DNS entries have been deleted", "component", service.ExtensionServiceName, "namespace", namespace)
			return &controllererror.RequeueAfterError{
				Cause:        fmt.Errorf("waiting until shoot DNS entries have been deleted"),
				RequeueAfter: 20 * time.Second,
			}
		}
	}
	if err := managedresources.Delete(ctx, a.Client(), namespace, SeedResourcesName, false); err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := managedresources.WaitUntilDeleted(timeoutCtx, a.Client(), namespace, SeedResourcesName); err != nil {
		return err
	}

	secret := &corev1.Secret{}
	secret.SetName(service.SecretName)
	secret.SetNamespace(namespace)
	if err := a.Client().Delete(ctx, secret); client.IgnoreNotFound(err) != nil {
		return err
	}

	return nil
}

func (a *actuator) cleanupShootDNSEntries(helper *common.ShootDNSEntriesHelper) error {
	cluster, err := helper.GetCluster()
	if err != nil {
		return err
	}
	err = a.createOrUpdateSeedResources(helper.Context(), cluster, helper.Extension(), false, false)
	if err != nil {
		return err
	}
	err = helper.DeleteAll()
	if err != nil {
		return err
	}
	return nil
}

func (a *actuator) createOrUpdateShootResources(ctx context.Context, cluster *controller.Cluster, namespace string) error {
	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion("apiextensions.k8s.io/v1beta1")
	crd.SetKind("CustomResourceDefinition")
	if err := a.Client().Get(ctx, client.ObjectKey{Name: "dnsentries.dns.gardener.cloud"}, crd); err != nil {
		return errors.Wrap(err, "could not get crd dnsentries.dns.gardener.cloud")
	}
	crd.SetResourceVersion("")
	crd.SetUID("")
	crd.SetCreationTimestamp(metav1.Time{})
	crd.SetGeneration(0)
	if err := managedresources.CreateFromUnstructured(ctx, a.Client(), namespace, KeptShootResourcesName, false, "", []*unstructured.Unstructured{crd}, true, nil); err != nil {
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
	injectedLabels := map[string]string{v1beta1constants.ShootNoCleanup: "true"}

	return a.createOrUpdateManagedResource(ctx, namespace, ShootResourcesName, "", renderer, service.ShootChartName, chartValues, injectedLabels)
}

func (a *actuator) deleteShootResources(ctx context.Context, namespace string) error {
	if err := managedresources.Delete(ctx, a.Client(), namespace, ShootResourcesName, false); err != nil {
		return err
	}
	if err := managedresources.Delete(ctx, a.Client(), namespace, KeptShootResourcesName, false); err != nil {
		return err
	}

	timeoutCtx1, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := managedresources.WaitUntilDeleted(timeoutCtx1, a.Client(), namespace, ShootResourcesName); err != nil {
		return err
	}

	timeoutCtx2, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return managedresources.WaitUntilDeleted(timeoutCtx2, a.Client(), namespace, KeptShootResourcesName)
}

func (a *actuator) createKubeconfig(ctx context.Context, namespace string) (*corev1.Secret, error) {
	certConfig := secrets.CertificateSecretConfig{
		Name:       service.SecretName,
		CommonName: service.UserName,
	}
	return util.GetOrCreateShootKubeconfig(ctx, a.Client(), certConfig, namespace)
}

func (a *actuator) createOrUpdateManagedResource(ctx context.Context, namespace, name, class string, renderer chartrenderer.Interface, chartName string, chartValues map[string]interface{}, injectedLabels map[string]string) error {
	chartPath := filepath.Join(service.ChartsPath, chartName)
	chart, err := renderer.Render(chartPath, chartName, namespace, chartValues)
	if err != nil {
		return err
	}

	data := map[string][]byte{chartName: chart.Manifest()}
	keepObjects := false
	forceOverwriteAnnotations := false
	return managedresources.Create(ctx, a.Client(), namespace, name, false, class, data, &keepObjects, injectedLabels, &forceOverwriteAnnotations)
}

// seedSettingShootDNSEnabled returns true if the 'shoot dns' setting is enabled.
func seedSettingShootDNSEnabled(settings *gardencorev1beta1.SeedSettings) bool {
	return settings == nil || settings.ShootDNS == nil || settings.ShootDNS.Enabled
}

func (a *actuator) OwnerName(namespace string) string {
	return fmt.Sprintf("%s-%s", OwnerName, namespace)
}

func (a *actuator) oldShootId(namespace string) string {
	return fmt.Sprintf("%s.gardener.cloud/%s", a.Config().GardenID, namespace)
}
