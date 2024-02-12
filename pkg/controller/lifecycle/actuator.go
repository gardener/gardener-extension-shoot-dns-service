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
	_ "embed"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	"github.com/gardener/gardener/extensions/pkg/util"
	gardencore "github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/controllerutils"
	reconcilerutils "github.com/gardener/gardener/pkg/controllerutils/reconciler"
	"github.com/gardener/gardener/pkg/extensions"
	"github.com/gardener/gardener/pkg/utils/chart"
	"github.com/gardener/gardener/pkg/utils/flow"
	gutil "github.com/gardener/gardener/pkg/utils/gardener"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/managedresources"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-extension-shoot-dns-service/charts"
	"github.com/gardener/gardener-extension-shoot-dns-service/imagevector"
	apisservice "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/validation"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/common"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
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
	// DNSProviderRoleAdditional is a constant for additionally managed DNS providers.
	DNSProviderRoleAdditional = "managed-dns-provider"
	// DNSRealmAnnotation is the annotation key for restricting provider access for shoot DNS entries
	DNSRealmAnnotation = "dns.gardener.cloud/realms"
	// ShootDNSServiceMaintainerAnnotation is the annotation key for marking a DNS providers a managed by shoot-dns-service
	ShootDNSServiceMaintainerAnnotation = "service.dns.extensions.gardener.cloud/maintainer"
	// ExternalDNSProviderName is the name of the external DNS provider
	ExternalDNSProviderName = "external"
	// ShootDNSServiceUseRemoteDefaultDomainLabel is the label key for marking a seed to use the remote DNS-provider for the default domain
	ShootDNSServiceUseRemoteDefaultDomainLabel = "service.dns.extensions.gardener.cloud/use-remote-default-domain"
)

// dnsAnnotationCRD contains the contents of the dnsAnnotationCRD.yaml file.
//
//go:embed dnsAnnotationCRD.yaml
var dnsAnnotationCRD string

// NewActuator returns an actuator responsible for Extension resources.
func NewActuator(mgr manager.Manager, chartApplier kubernetes.ChartApplier, chartRenderer chartrenderer.Interface, config config.DNSServiceConfig) extension.Actuator {
	return &actuator{
		Env:      common.NewEnv(ActuatorName, mgr, config),
		applier:  chartApplier,
		renderer: chartRenderer,
		decoder:  serializer.NewCodecFactory(mgr.GetScheme(), serializer.EnableStrict).UniversalDecoder(),
	}
}

type actuator struct {
	*common.Env
	applier  kubernetes.ChartApplier
	renderer chartrenderer.Interface
	decoder  runtime.Decoder
}

// Reconcile the Extension resource.
func (a *actuator) Reconcile(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	cluster, err := controller.GetCluster(ctx, a.Client(), ex.Namespace)
	if err != nil {
		return err
	}

	dnsConfig, err := a.extractDNSConfig(ex)
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

	// Shoots that don't specify a DNS domain don't get an DNS service
	if cluster.Shoot.Spec.DNS == nil {
		log.Info("DNS domain is not specified, therefore no shoot dns service is installed", "shoot", ex.Namespace)
		return a.Delete(ctx, log, ex)
	}

	if err := a.createOrUpdateShootResources(ctx, dnsConfig, cluster, ex.Namespace); err != nil {
		return err
	}
	if err := a.createOrUpdateSeedResources(ctx, dnsConfig, cluster, ex, !resurrection, true); err != nil {
		return err
	}
	return a.createOrUpdateDNSProviders(ctx, log, dnsConfig, cluster, ex)
}

func (a *actuator) extractDNSConfig(ex *extensionsv1alpha1.Extension) (*apisservice.DNSConfig, error) {
	dnsConfig := &apisservice.DNSConfig{}
	if ex.Spec.ProviderConfig != nil {
		if _, _, err := a.decoder.Decode(ex.Spec.ProviderConfig.Raw, nil, dnsConfig); err != nil {
			return nil, fmt.Errorf("failed to decode provider config: %+v", err)
		}
		if errs := validation.ValidateDNSConfig(dnsConfig, nil); len(errs) > 0 {
			return nil, errs.ToAggregate()
		}
	}
	return dnsConfig, nil
}

func (a *actuator) ResurrectFrom(ctx context.Context, ex *extensionsv1alpha1.Extension) (bool, error) {
	owner := &dnsv1alpha1.DNSOwner{}

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
	err = a.GetObject(ctx, client.ObjectKey{Namespace: ex.Namespace, Name: SeedResourcesName}, &resourcesv1alpha1.ManagedResource{})
	if err == nil || !k8serr.IsNotFound(err) {
		// a potentially missing DNSOwner object will be reconciled by resource manager
		return false, err
	}

	handler.Infof("resources object not found, also -> trying to resurrect DNS entries before setting up new owner")

	found, err := handler.ShootDNSEntriesHelper().List()
	if err != nil {
		return true, err
	}
	names := sets.Set[string]{}
	for _, item := range found {
		names.Insert(item.Name)
	}
	var lasterr error
	for _, item := range handler.StateItems() {
		if names.Has(item.Name) {
			continue
		}
		obj := &dnsv1alpha1.DNSEntry{
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
func (a *actuator) Delete(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	return a.delete(ctx, log, ex, false)
}

// ForceDelete the Extension resource.
func (a *actuator) ForceDelete(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	// try to delete managed DNS entries normally first
	if err := a.deleteManagedDNSEntries(ctx, ex); err != nil {
		// ignore failed deletion of DNSEntries
		if _, ok := err.(*reconcilerutils.RequeueAfterError); !ok {
			return err
		}
	}

	cluster, err := controller.GetCluster(ctx, a.Client(), ex.Namespace)
	if err != nil {
		return err
	}

	if err := a.deleteSeedResources(ctx, log, cluster, ex, false, true); err != nil {
		return err
	}

	entriesHelper := common.NewShootDNSEntriesHelper(ctx, a.Client(), ex)
	if err := entriesHelper.ForceDeleteAll(); err != nil {
		return fmt.Errorf("force deletion of DNSEntries failed: %w", err)
	}

	if a.isManagingDNSProviders(cluster.Shoot.Spec.DNS) {
		// no forced deletion of providers needed, as they can be deleted normally as soon as there are no DNSEntries anymore
		if err := a.deleteDNSProviders(ctx, log, ex.Namespace); err != nil {
			return err
		}
	}

	return nil
}

func (a *actuator) delete(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension, migrate bool) error {
	cluster, err := controller.GetCluster(ctx, a.Client(), ex.Namespace)
	if err != nil {
		return err
	}

	if err := a.deleteSeedResources(ctx, log, cluster, ex, migrate, false); err != nil {
		return err
	}
	return a.deleteShootResources(ctx, ex.Namespace)
}

// Restore the Extension resource.
func (a *actuator) Restore(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	return a.Reconcile(ctx, log, ex)
}

// Migrate the Extension resource.
func (a *actuator) Migrate(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	// Keep objects for shoot managed resources so that they are not deleted from the shoot during the migration
	if err := managedresources.SetKeepObjects(ctx, a.Client(), ex.GetNamespace(), ShootResourcesName, true); err != nil {
		return err
	}

	return a.delete(ctx, log, ex, true)
}

func (a *actuator) isManagingDNSProviders(dns *gardencorev1beta1.DNS) bool {
	return a.Config().ManageDNSProviders && dns != nil && dns.Domain != nil
}

func (a *actuator) isHibernated(cluster *controller.Cluster) bool {
	hibernation := cluster.Shoot.Spec.Hibernation
	return hibernation != nil && hibernation.Enabled != nil && *hibernation.Enabled
}

func (a *actuator) createOrUpdateSeedResources(ctx context.Context, dnsconfig *apisservice.DNSConfig, cluster *controller.Cluster, ex *extensionsv1alpha1.Extension,
	refresh bool, deploymentEnabled bool) error {
	var err error
	namespace := ex.Namespace

	handler, err := common.NewStateHandler(ctx, a.Env, ex, refresh)
	if err != nil {
		return err
	}
	err = handler.Update("refresh")
	if err != nil {
		return err
	}

	shootID, creatorLabelValue, err := handler.ShootDNSEntriesHelper().ShootID()
	if err != nil {
		return err
	}

	seedID := a.Config().SeedID
	if seedID == "" {
		if cluster.Seed.Status.ClusterIdentity == nil {
			return fmt.Errorf("missing 'seed.status.clusterIdentity' in cluster")
		}
		seedID = *cluster.Seed.Status.ClusterIdentity
		a.Config().SeedID = seedID
	}

	replicas := 1
	if !deploymentEnabled || a.isHibernated(cluster) {
		replicas = 0
	}
	shootActive := !common.IsMigrating(ex)

	chartValues := map[string]interface{}{
		"serviceName":                      service.ServiceName,
		"genericTokenKubeconfigSecretName": extensions.GenericTokenKubeconfigSecretNameFromCluster(cluster),
		"replicas":                         controller.GetReplicas(cluster, replicas),
		"creatorLabelValue":                creatorLabelValue,
		"shootId":                          shootID,
		"seedId":                           seedID,
		"dnsClass":                         a.Config().DNSClass,
		"dnsProviderReplication": map[string]interface{}{
			"enabled": a.replicateDNSProviders(dnsconfig),
		},
		"dnsOwner":    a.OwnerName(namespace),
		"shootActive": shootActive,
	}

	if err := gutil.NewShootAccessSecret(service.ShootAccessSecretName, namespace).Reconcile(ctx, a.Client()); err != nil {
		return err
	}
	chartValues["targetClusterSecret"] = gutil.SecretNamePrefixShootAccess + service.ShootAccessSecretName

	chartValues, err = chart.InjectImages(chartValues, imagevector.ImageVector(), []string{service.ImageName})
	if err != nil {
		return fmt.Errorf("failed to find image version for %s: %v", service.ImageName, err)
	}

	a.Info("Component is being applied", "component", service.ExtensionServiceName, "namespace", namespace)
	return a.createOrUpdateManagedResource(ctx, namespace, SeedResourcesName, "seed", a.renderer, service.SeedChartName, chartValues, nil)
}

func (a *actuator) createOrUpdateDNSProviders(ctx context.Context, log logr.Logger, dnsconfig *apisservice.DNSConfig,
	cluster *controller.Cluster, ex *extensionsv1alpha1.Extension) error {
	if !a.isManagingDNSProviders(cluster.Shoot.Spec.DNS) {
		return nil
	}

	var err, result error
	namespace := ex.Namespace
	deployers := map[string]component.DeployWaiter{}

	if !a.isHibernated(cluster) {
		external, err := a.prepareDefaultExternalDNSProvider(ctx, dnsconfig, namespace, cluster)
		if err != nil {
			return err
		}

		resources := cluster.Shoot.Spec.Resources
		providers := map[string]*dnsv1alpha1.DNSProvider{}
		providers[ExternalDNSProviderName] = nil // remember for deletion
		if external != nil {
			providers[ExternalDNSProviderName] = buildDNSProvider(external, namespace, ExternalDNSProviderName, "")
		}

		result = a.addAdditionalDNSProviders(providers, ctx, result, dnsconfig, namespace, resources)

		for name, p := range providers {
			var dw component.DeployWaiter
			if p != nil {
				dw = NewProviderDeployWaiter(log, a.Client(), p)
			}
			deployers[name] = dw
		}
	} else {
		err := a.deleteManagedDNSEntries(ctx, ex)
		if err != nil {
			return err
		}
	}

	err = a.addCleanupOfOldAdditionalProviders(deployers, ctx, log, namespace)
	if err != nil {
		result = multierror.Append(result, err)
	}

	err = a.deployDNSProviders(ctx, deployers)
	if err != nil {
		result = multierror.Append(result, err)
	}
	return result
}

// addCleanupOfOldAdditionalProviders adds destroy DeployWaiter to clean up old orphaned additional providers
func (a *actuator) addCleanupOfOldAdditionalProviders(dnsProviders map[string]component.DeployWaiter, ctx context.Context, log logr.Logger, namespace string) error {
	providerList := &dnsv1alpha1.DNSProviderList{}
	if err := a.Client().List(
		ctx,
		providerList,
		client.InNamespace(namespace),
		client.MatchingLabels{v1beta1constants.GardenRole: DNSProviderRoleAdditional},
	); err != nil {
		return err
	}

	for _, provider := range providerList.Items {
		if _, ok := dnsProviders[provider.Name]; !ok {
			p := provider
			dnsProviders[provider.Name] = component.OpDestroyAndWait(NewProviderDeployWaiter(
				log,
				a.Client(),
				&p,
			))
		}
	}

	if dw, ok := dnsProviders[ExternalDNSProviderName]; dw == nil && ok {
		// delete non-migrated non-default external DNS provider if it exists
		provider := &dnsv1alpha1.DNSProvider{}
		if err := a.Client().Get(
			ctx,
			kutil.Key(namespace, ExternalDNSProviderName),
			provider,
		); err == nil {
			dnsProviders[provider.Name] = component.OpDestroyAndWait(NewProviderDeployWaiter(
				log,
				a.Client(),
				provider,
			))
		}
	}

	return nil
}

// deployDNSProviders deploys the specified DNS providers in the shoot namespace of the seed.
func (a *actuator) deployDNSProviders(ctx context.Context, dnsProviders map[string]component.DeployWaiter) error {
	if len(dnsProviders) == 0 {
		return nil
	}
	fns := make([]flow.TaskFn, 0, len(dnsProviders))

	for _, p := range dnsProviders {
		if p != nil {
			deployWaiter := p
			fns = append(fns, func(ctx context.Context) error {
				return component.OpWait(deployWaiter).Deploy(ctx)
			})
		}
	}

	return flow.Parallel(fns...)(ctx)
}

func (a *actuator) addAdditionalDNSProviders(providers map[string]*dnsv1alpha1.DNSProvider, ctx context.Context, result error,
	dnsconfig *apisservice.DNSConfig, namespace string, resources []gardencorev1beta1.NamedResourceReference) error {
	for i, provider := range dnsconfig.Providers {
		p := provider

		providerType := p.Type
		if providerType == nil {
			result = multierror.Append(result, fmt.Errorf("dns provider[%d] doesn't specify a type", i))
			continue
		}

		if *providerType == gardencore.DNSUnmanaged {
			a.Logger.Info(fmt.Sprintf("Skipping deployment of DNS provider[%d] since it specifies type %q", i, gardencore.DNSUnmanaged))
			continue
		}

		mappedSecretName, err := lookupReference(resources, p.SecretName, i)
		if err != nil {
			result = multierror.Append(result, err)
			continue
		}

		providerName := fmt.Sprintf("%s-%s", *providerType, *p.SecretName)
		providers[providerName] = nil

		secret := &corev1.Secret{}
		if err := a.Client().Get(
			ctx,
			kutil.Key(namespace, mappedSecretName),
			secret,
		); err != nil {
			result = multierror.Append(result, fmt.Errorf("could not get dns provider[%d] secret %q -> %q: %w", i, *p.SecretName, mappedSecretName, err))
			continue
		}

		providers[providerName] = buildDNSProvider(&p, namespace, providerName, mappedSecretName)
	}
	return result
}

func buildDNSProvider(p *apisservice.DNSProvider, namespace, name string, mappedSecretName string) *dnsv1alpha1.DNSProvider {
	var includeDomains, excludeDomains, includeZones, excludeZones []string
	if domains := p.Domains; domains != nil {
		includeDomains = domains.Include
		excludeDomains = domains.Exclude
	}
	if zones := p.Zones; zones != nil {
		includeZones = zones.Include
		excludeZones = zones.Exclude
	}
	secretName := *p.SecretName
	if mappedSecretName != "" {
		secretName = mappedSecretName
	}
	return &dnsv1alpha1.DNSProvider{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Labels:      map[string]string{v1beta1constants.GardenRole: DNSProviderRoleAdditional},
			Annotations: enableDNSProviderForShootDNSEntries(namespace),
		},
		Spec: dnsv1alpha1.DNSProviderSpec{
			Type:           *p.Type,
			ProviderConfig: nil,
			SecretRef: &corev1.SecretReference{
				Name:      secretName,
				Namespace: namespace,
			},
			Domains: &dnsv1alpha1.DNSSelection{
				Include: includeDomains,
				Exclude: excludeDomains,
			},
			Zones: &dnsv1alpha1.DNSSelection{
				Include: includeZones,
				Exclude: excludeZones,
			},
		},
	}
}

func lookupReference(resources []gardencorev1beta1.NamedResourceReference, secretName *string, index int) (string, error) {
	if secretName == nil {
		return "", fmt.Errorf("dns provider[%d] doesn't specify a secretName", index)
	}

	for _, res := range resources {
		if res.Name == *secretName {
			return v1beta1constants.ReferencedResourcesPrefix + res.ResourceRef.Name, nil
		}
	}

	return "", fmt.Errorf("dns provider[%d] secretName %s not found in referenced resources", index, *secretName)
}

func (a *actuator) prepareDefaultExternalDNSProvider(ctx context.Context, _ *apisservice.DNSConfig, namespace string, cluster *controller.Cluster) (*apisservice.DNSProvider, error) {
	for _, provider := range cluster.Shoot.Spec.DNS.Providers {
		if provider.Primary != nil && *provider.Primary {
			return nil, nil
		}
	}

	if a.useRemoteDefaultDomain(cluster) {
		secretName, err := a.copyRemoteDefaultDomainSecret(ctx, namespace)
		if err != nil {
			return nil, err
		}
		remoteType := "remote"
		return &apisservice.DNSProvider{
			Domains: &apisservice.DNSIncludeExclude{
				Include: []string{*cluster.Shoot.Spec.DNS.Domain},
				Exclude: []string{"api." + *cluster.Shoot.Spec.DNS.Domain}, // exclude external kube-apiserver domain
			},
			SecretName: &secretName,
			Type:       &remoteType,
		}, nil
	}

	secretRef, providerType, zone, err := GetSecretRefFromDNSRecordExternal(ctx, a.Client(), namespace, cluster.Shoot.Name)
	if err != nil || secretRef == nil {
		return nil, err
	}
	provider := &apisservice.DNSProvider{
		Domains: &apisservice.DNSIncludeExclude{
			Include: []string{*cluster.Shoot.Spec.DNS.Domain},
			Exclude: []string{"api." + *cluster.Shoot.Spec.DNS.Domain}, // exclude external kube-apiserver domain
		},
		SecretName: &secretRef.Name,
		Type:       &providerType,
	}
	if zone != nil {
		provider.Zones = &apisservice.DNSIncludeExclude{
			Include: []string{*zone},
		}
	}
	return provider, nil
}

func (a *actuator) useRemoteDefaultDomain(cluster *controller.Cluster) bool {
	if a.Config().RemoteDefaultDomainSecret != nil && cluster.Seed.Labels != nil {
		annot, ok := cluster.Seed.Labels[ShootDNSServiceUseRemoteDefaultDomainLabel]
		return ok && annot == "true"
	}
	return false
}

func (a *actuator) copyRemoteDefaultDomainSecret(ctx context.Context, namespace string) (string, error) {
	secretOrg := &corev1.Secret{}
	err := a.Client().Get(ctx, *a.Config().RemoteDefaultDomainSecret, secretOrg)
	if err != nil {
		return "", err
	}

	secret := &corev1.Secret{}
	secret.Namespace = namespace
	secret.Name = "shoot-dns-service-remote-default-domains"
	_, err = controllerutils.CreateOrGetAndMergePatch(ctx, a.Client(), secret, func() error {
		secret.Data = secretOrg.Data
		return nil
	})
	if err != nil {
		return "", err
	}
	return secret.Name, err
}

func (a *actuator) replicateDNSProviders(dnsconfig *apisservice.DNSConfig) bool {
	if dnsconfig != nil && dnsconfig.DNSProviderReplication != nil {
		return dnsconfig.DNSProviderReplication.Enabled
	}
	return a.Config().ReplicateDNSProviders
}

func (a *actuator) deleteSeedResources(ctx context.Context, log logr.Logger, cluster *controller.Cluster, ex *extensionsv1alpha1.Extension, migrate, force bool) error {
	namespace := ex.Namespace
	a.Info("Component is being deleted", "component", service.ExtensionServiceName, "namespace", namespace)

	if !force {
		if !migrate {
			err := a.deleteManagedDNSEntries(ctx, ex)
			if err != nil {
				return err
			}
		}

		if a.isManagingDNSProviders(cluster.Shoot.Spec.DNS) {
			if err := a.deleteDNSProviders(ctx, log, namespace); err != nil {
				return err
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

	return kutil.DeleteObject(ctx, a.Client(), &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: gutil.SecretNamePrefixShootAccess + service.ShootAccessSecretName, Namespace: namespace}})
}

func (a *actuator) deleteManagedDNSEntries(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
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
			return fmt.Errorf("cleanupShootDNSEntries failed: %w", err)
		}
		a.Info("Waiting until all shoot DNS entries have been deleted", "component", service.ExtensionServiceName, "namespace", ex.Namespace)
		for i := 0; i < 6; i++ {
			time.Sleep(5 * time.Second)
			list, err = entriesHelper.List()
			if err != nil {
				break
			}
			if len(list) == 0 {
				return nil
			}
		}
		details := a.collectProviderDetailsOnDeletingDNSEntries(ctx, list)
		return &reconcilerutils.RequeueAfterError{
			Cause:        fmt.Errorf("waiting until shoot DNS entries have been deleted: %s", details),
			RequeueAfter: 15 * time.Second,
		}
	}
	return nil
}

func (a *actuator) collectProviderDetailsOnDeletingDNSEntries(ctx context.Context, list []dnsv1alpha1.DNSEntry) string {
	providers := sets.NewString()
	for _, item := range list {
		if item.DeletionTimestamp != nil && item.DeletionTimestamp.Time.Add(1*time.Minute).Before(time.Now()) {
			if item.Status.Provider != nil {
				providers.Insert(*item.Status.Provider)
			} else {
				providers.Insert("")
			}
		}
	}
	var status []string
	for k := range providers {
		if k == "" {
			status = append(status, "no suitable provider")
			continue
		}
		namespace, name := kutil.ParseObjectName(k)
		if namespace == "" {
			status = append(status, fmt.Sprintf("unknown provider name: %s", k))
			continue
		}
		provider := &dnsv1alpha1.DNSProvider{}
		if err := a.Client().Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, provider); err != nil {
			status = append(status, fmt.Sprintf("error on retrieving status of provider %s: %s", k, err))
			continue
		}
		status = append(status, fmt.Sprintf("provider %s has status: %s", name, ptr.Deref(provider.Status.Message, "unknwon")))
	}
	return strings.Join(status, ", ")
}

// deleteDNSProviders deletes the external and additional providers
func (a *actuator) deleteDNSProviders(ctx context.Context, log logr.Logger, namespace string) error {
	dnsProviders := map[string]component.DeployWaiter{}

	if err := a.addCleanupOfOldAdditionalProviders(dnsProviders, ctx, log, namespace); err != nil {
		return err
	}

	return a.deployDNSProviders(ctx, dnsProviders)
}

func (a *actuator) cleanupShootDNSEntries(helper *common.ShootDNSEntriesHelper) error {
	cluster, err := helper.GetCluster()
	if err != nil {
		return err
	}
	dnsconfig, err := a.extractDNSConfig(helper.Extension())
	if err != nil {
		return err
	}
	err = a.createOrUpdateSeedResources(helper.Context(), dnsconfig, cluster, helper.Extension(), false, false)
	if err != nil {
		return err
	}

	return helper.DeleteAll()
}

func (a *actuator) createOrUpdateShootResources(ctx context.Context, dnsconfig *apisservice.DNSConfig, cluster *controller.Cluster, namespace string) error {
	crd := &unstructured.Unstructured{}
	crd.SetAPIVersion(apiextensionsv1.SchemeGroupVersion.String())
	crd.SetKind("CustomResourceDefinition")
	if err := a.Client().Get(ctx, client.ObjectKey{Name: "dnsentries.dns.gardener.cloud"}, crd); err != nil {
		return fmt.Errorf("could not get crd dnsentries.dns.gardener.cloud: %w", err)
	}
	cleanCRD(crd)

	crd2 := &unstructured.Unstructured{}
	dec := yaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
	_, _, err := dec.Decode([]byte(dnsAnnotationCRD), nil, crd2)
	if err != nil {
		return fmt.Errorf("could not unmarshal dnsannotation.dns.gardener.cloud crd: %w", err)
	}

	replicateDNSProviders := a.replicateDNSProviders(dnsconfig)
	crds := []*unstructured.Unstructured{crd, crd2}
	if replicateDNSProviders {
		crd3 := &unstructured.Unstructured{}
		crd3.SetAPIVersion(crd.GetAPIVersion())
		crd3.SetKind(crd.GetKind())
		if err := a.Client().Get(ctx, client.ObjectKey{Name: "dnsproviders.dns.gardener.cloud"}, crd3); err != nil {
			return fmt.Errorf("could not get crd dnsproviders.dns.gardener.cloud: %w", err)
		}
		cleanCRD(crd3)
		crds = append(crds, crd3)
	}

	for _, c := range crds {
		labels := c.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		labels[v1beta1constants.ShootNoCleanup] = "true"
		c.SetLabels(labels)
	}

	if err = managedresources.CreateFromUnstructured(ctx, a.Client(), namespace, KeptShootResourcesName, false, "", crds, true, nil); err != nil {
		return fmt.Errorf("could not create managed resource %s: %w", KeptShootResourcesName, err)
	}

	renderer, err := util.NewChartRendererForShoot(cluster.Shoot.Spec.Kubernetes.Version)
	if err != nil {
		return fmt.Errorf("could not create chart renderer: %w", err)
	}

	chartValues := map[string]interface{}{
		"serviceName": service.ServiceName,
		"dnsProviderReplication": map[string]interface{}{
			"enabled": replicateDNSProviders,
		},
		"shootAccessServiceAccountName": service.ShootAccessServiceAccountName,
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

func (a *actuator) createOrUpdateManagedResource(ctx context.Context, namespace, name, class string, renderer chartrenderer.Interface, chartName string, chartValues map[string]interface{}, injectedLabels map[string]string) error {
	chartPath := filepath.Join(charts.ChartsPath, chartName)
	chart, err := renderer.RenderEmbeddedFS(charts.Internal, chartPath, chartName, namespace, chartValues)
	if err != nil {
		return err
	}

	data := map[string][]byte{chartName: chart.Manifest()}
	keepObjects := false
	forceOverwriteAnnotations := false
	return managedresources.Create(ctx, a.Client(), namespace, name, nil, false, class, data, &keepObjects, injectedLabels, &forceOverwriteAnnotations)
}

func (a *actuator) OwnerName(namespace string) string {
	return fmt.Sprintf("%s-%s", OwnerName, namespace)
}

func cleanCRD(crd *unstructured.Unstructured) {
	crd.SetResourceVersion("")
	crd.SetUID("")
	crd.SetCreationTimestamp(metav1.Time{})
	crd.SetGeneration(0)
	crd.SetManagedFields(nil)
	annotations := crd.GetAnnotations()
	if annotations != nil {
		delete(annotations, "kubectl.kubernetes.io/last-applied-configuration")
	}
	crd.SetAnnotations(annotations)
}

func enableDNSProviderForShootDNSEntries(seedNamespace string) map[string]string {
	return map[string]string{DNSRealmAnnotation: fmt.Sprintf("%s,", seedNamespace)}
}
