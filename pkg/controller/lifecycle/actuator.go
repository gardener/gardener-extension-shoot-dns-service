// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	extensionsconfigv1alpha1 "github.com/gardener/gardener/extensions/pkg/apis/config/v1alpha1"
	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	"github.com/gardener/gardener/extensions/pkg/util"
	gardencore "github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
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
	"github.com/gardener/gardener/pkg/utils/retry"
	"github.com/go-logr/logr"
	"github.com/hashicorp/go-multierror"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	k8serr "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/gardener/gardener-extension-shoot-dns-service/charts"
	"github.com/gardener/gardener-extension-shoot-dns-service/imagevector"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/helper"
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
	// DropDNSEntriesStateOnMigration is the annotation key for dropping the state of DNSEntries during migration.
	// Activated by setting the annotation value to "true".
	// This may be helpful if the state is too large to be stored in the extension state.
	// In this case, it may be a fall-back option, to set this annotation value to "true" and run the migration without
	// DNSEntries in the extension status state. Nothing should be lost, but if source objects are deleted during the migration,
	// the deletion of the DNS records cannot be guaranteed.
	DropDNSEntriesStateOnMigration = "drop-dns-entries-state-on-migration"

	// NextGenerationTargetClass is the target class for the next generation DNS controller.
	NextGenerationTargetClass = "gardendns-next-gen"
)

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

	// Shoots that don't specify a DNS domain don't get a DNS service
	if cluster.Shoot.Spec.DNS == nil {
		log.Info("DNS domain is not specified, therefore no shoot dns service is installed", "shoot", ex.Namespace)
		return a.Delete(ctx, log, ex)
	}

	if ex.Status.State != nil && common.IsRestoring(ex) {
		if err := a.ResurrectFrom(ctx, ex); err != nil {
			return err
		}
	}

	if err := a.createOrUpdateShootResources(ctx, dnsConfig, cluster, ex.Namespace); err != nil {
		return err
	}
	if err := a.createOrUpdateSeedResources(ctx, dnsConfig, cluster, ex, true); err != nil {
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
		if errs := validation.ValidateDNSConfig(dnsConfig, nil, nil); len(errs) > 0 {
			return nil, errs.ToAggregate()
		}
	}
	return dnsConfig, nil
}

func (a *actuator) ResurrectFrom(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	handler, err := common.NewStateHandler(ctx, a.Env, ex)
	if err != nil {
		return err
	}

	handler.Info("resurrect DNS entries", "namespace", ex.Namespace, "name", ex.Name)

	found, err := handler.ShootDNSEntriesHelper().List()
	if err != nil {
		return err
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

	return lasterr
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
	if err := a.waitForEntryReconciliation(ctx, log, ex); err != nil {
		return err
	}
	return a.Reconcile(ctx, log, ex)
}

// Migrate the Extension resource.
func (a *actuator) Migrate(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	// Keep objects for shoot managed resources so that they are not deleted from the shoot during the migration
	if err := managedresources.SetKeepObjects(ctx, a.Client(), ex.GetNamespace(), ShootResourcesName, true); err != nil {
		return err
	}

	if err := a.ignoreDNSEntriesForMigration(ctx, ex); err != nil {
		return err
	}

	if ex.Annotations[DropDNSEntriesStateOnMigration] == "true" {
		if err := a.ensureStateDropped(ctx, ex); err != nil {
			return err
		}
	} else {
		if err := a.ensureStateRefreshed(ctx, ex); err != nil {
			return err
		}
	}

	return a.delete(ctx, log, ex, true)
}

func (a *actuator) ignoreDNSEntriesForMigration(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	entriesHelper := common.NewShootDNSEntriesHelper(ctx, a.Client(), ex)
	list, err := entriesHelper.List()
	if err != nil {
		return err
	}
	for _, entry := range list {
		patch := client.MergeFrom(entry.DeepCopy())
		if entry.Annotations == nil {
			entry.Annotations = map[string]string{}
		}
		entry.Annotations[dns.AnnotationHardIgnore] = "true"
		if err := client.IgnoreNotFound(a.Client().Patch(ctx, &entry, patch)); err != nil {
			return fmt.Errorf("failed to ignore DNS entry %q: %w", entry.Name, err)
		}
	}
	return nil
}

func (a *actuator) waitForEntryReconciliation(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	entriesHelper := common.NewShootDNSEntriesHelper(ctx, a.Client(), ex)
	list, err := entriesHelper.List()
	if err != nil {
		return err
	}

	// annotate all entries with gardener.cloud/operation=reconcile
	for _, entry := range list {
		patch := client.MergeFrom(entry.DeepCopy())
		if entry.Annotations == nil {
			entry.Annotations = map[string]string{}
		}
		entry.Annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
		delete(entry.Annotations, dns.AnnotationHardIgnore) // should not be needed as the DNSEntries have been recreated, but just to be sure
		if err := client.IgnoreNotFound(a.Client().Patch(ctx, &entry, patch)); err != nil {
			return fmt.Errorf("failed to revert ignore DNS entry %q: %w", entry.Name, err)
		}
	}

	// wait for all entries to be reconciled i.e., gardener.cloud/operation annotation is removed
	start := time.Now()
	for _, entry := range list {
		for {
			if err := a.Client().Get(ctx, client.ObjectKeyFromObject(&entry), &entry); err != nil {
				return err
			}
			if _, ok := entry.Annotations[v1beta1constants.GardenerOperation]; !ok {
				log.Info("DNS entry reconciled", "entry", entry.Name)
				break
			}
			if time.Since(start) > 3*time.Minute {
				return fmt.Errorf("timeout waiting for DNS entry %q to be reconciled", entry.Name)
			}
			time.Sleep(1 * time.Second)
		}
	}

	return nil
}

func (a *actuator) isManagingDNSProviders(dns *gardencorev1beta1.DNS) bool {
	return a.Config().ManageDNSProviders && dns != nil && dns.Domain != nil
}

func (a *actuator) isHibernated(cluster *controller.Cluster) bool {
	hibernation := cluster.Shoot.Spec.Hibernation
	return hibernation != nil && hibernation.Enabled != nil && *hibernation.Enabled
}

func (a *actuator) createOrUpdateSeedResources(ctx context.Context, dnsconfig *apisservice.DNSConfig, cluster *controller.Cluster, ex *extensionsv1alpha1.Extension,
	deploymentEnabled bool) error {
	var err error
	namespace := ex.Namespace

	a.Info("Creating/updating seed resources", "namespace", namespace)
	if !common.IsRestoring(ex) {
		if err := a.ensureStateDropped(ctx, ex); err != nil {
			return err
		}
	}

	shootID, creatorLabelValue, err := common.ShootID(cluster)
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

	chartValues := map[string]any{
		"serviceName":                      service.ServiceName,
		"genericTokenKubeconfigSecretName": extensions.GenericTokenKubeconfigSecretNameFromCluster(cluster),
		"replicas":                         controller.GetReplicas(cluster, replicas),
		"creatorLabelValue":                creatorLabelValue,
		"shootId":                          shootID,
		"seedId":                           seedID,
		"dnsClass":                         a.Config().DNSClass,
		"dnsProviderReplication": map[string]any{
			"enabled": a.replicateDNSProviders(dnsconfig),
		},
		"nextGeneration": map[string]any{
			"enabled":  a.useNextGenerationController(dnsconfig),
			"dnsClass": NextGenerationTargetClass,
		},
	}

	if err := gutil.NewShootAccessSecret(service.ShootAccessSecretName, namespace).Reconcile(ctx, a.Client()); err != nil {
		return err
	}
	chartValues["targetClusterSecret"] = gutil.SecretNamePrefixShootAccess + service.ShootAccessSecretName

	chartValues, err = chart.InjectImages(chartValues, imagevector.ImageVector(), []string{service.ImageName, service.ImageNameNextGeneration})
	if err != nil {
		return fmt.Errorf("failed to find image version for %s and %s: %v", service.ImageName, service.ImageNameNextGeneration, err)
	}

	a.Info("Component is being applied", "component", service.ExtensionServiceName, "namespace", namespace)
	return a.createOrUpdateManagedResource(ctx, namespace, SeedResourcesName, "seed", a.renderer, service.SeedChartName, chartValues, nil)
}

func (a *actuator) ensureStateDropped(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	// The DNSEntries are not stored in the extension state, as they are only needed for control plane migration during the
	// restore step.
	handler, err := common.NewStateHandler(ctx, a.Env, ex)
	if err != nil {
		a.Info("ignoring state handler error", "error", err, "namespace", ex.Namespace)
	}
	handler.DropAllEntries()
	return handler.Update("cleanup")
}

func (a *actuator) ensureStateRefreshed(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	// The DNSEntries in the control plane are listed and stored in the extension state for migration.
	handler, err := common.NewStateHandler(ctx, a.Env, ex)
	if err != nil {
		a.Info("ignoring state handler error", "error", err, "namespace", ex.Namespace)
	}
	handler.Info("refreshing state", "err", err, "namespace", ex.Namespace, "name", ex.Name)
	if _, err = handler.Refresh(); err != nil {
		handler.Info("refreshing state failed", "err", err, "namespace", ex.Namespace, "name", ex.Name)
		return err
	}
	return handler.Update("refresh")
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

		var class *string
		if a.useNextGenerationController(dnsconfig) {
			class = ptr.To(NextGenerationTargetClass)
		}
		for name, p := range providers {
			var dw component.DeployWaiter
			if p != nil {
				dw = NewProviderDeployWaiter(log, a.Client(), p, class)
			}
			deployers[name] = dw
		}
	} else {
		err := a.deleteManagedDNSEntries(ctx, ex)
		if err != nil {
			return err
		}
	}

	err = a.addCleanupOfOldAdditionalProviders(deployers, ctx, log, namespace, true)
	if err != nil {
		result = multierror.Append(result, err)
	}

	err = a.deployDNSProviders(ctx, deployers)
	if err != nil {
		result = multierror.Append(result, err)
	}
	return result
}

func (a *actuator) useNextGenerationController(dnsconfig *apisservice.DNSConfig) bool {
	return ptr.Deref(dnsconfig.UseNextGenerationController, false)
}

// addCleanupOfOldAdditionalProviders adds destroy DeployWaiter to clean up old orphaned additional providers
func (a *actuator) addCleanupOfOldAdditionalProviders(dnsProviders map[string]component.DeployWaiter, ctx context.Context, log logr.Logger, namespace string, keepReplicatedProviders bool) error {
	providerList := &dnsv1alpha1.DNSProviderList{}
	if err := a.Client().List(
		ctx,
		providerList,
		client.InNamespace(namespace),
	); err != nil {
		return err
	}

	for _, provider := range providerList.Items {
		if !isAdditionalProvider(provider) && (keepReplicatedProviders || !isReplicatedProvider(provider)) {
			continue
		}
		if _, ok := dnsProviders[provider.Name]; !ok {
			p := provider
			dnsProviders[provider.Name] = component.OpDestroyAndWait(NewProviderDeployWaiter(
				log,
				a.Client(),
				&p,
				nil,
			))
		}
	}

	if dw, ok := dnsProviders[ExternalDNSProviderName]; dw == nil && ok {
		// delete non-migrated non-default external DNS provider if it exists
		provider := &dnsv1alpha1.DNSProvider{}
		if err := a.Client().Get(
			ctx,
			client.ObjectKey{Namespace: namespace, Name: ExternalDNSProviderName},
			provider,
		); err == nil {
			dnsProviders[provider.Name] = component.OpDestroyAndWait(NewProviderDeployWaiter(
				log,
				a.Client(),
				provider,
				nil,
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
			a.Info(fmt.Sprintf("Skipping deployment of DNS provider[%d] since it specifies type %q", i, gardencore.DNSUnmanaged))
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
			client.ObjectKey{Namespace: namespace, Name: mappedSecretName},
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

func (a *actuator) prepareDefaultExternalDNSProvider(ctx context.Context, dnsconfig *apisservice.DNSConfig, namespace string, cluster *controller.Cluster) (*apisservice.DNSProvider, error) {
	for _, provider := range cluster.Shoot.Spec.DNS.Providers {
		if provider.Primary != nil && *provider.Primary {
			return nil, nil
		}
	}

	if a.useRemoteDefaultDomain(cluster, dnsconfig) {
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

func (a *actuator) useRemoteDefaultDomain(cluster *controller.Cluster, dnsconfig *apisservice.DNSConfig) bool {
	if a.useNextGenerationController(dnsconfig) {
		// The next generation controller does not support remote default domain handling
		return false
	}
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

	// DNSEntries and DNSProvider are deleted after the seed resources have been deleted, so that the
	// shoot-dns-service deployment is already gone and cannot resurrect resources from the shoot cluster.
	if !force {
		if !migrate {
			err := a.deleteManagedDNSEntries(ctx, ex)
			if err != nil {
				return err
			}
			// need to remove finalizers from DNSEntries and DNSProviders on shoot as
			// shoot-dns-service is not running anymore on the seed
			if err := a.removeShootCustomResourcesFinalizersAndDeleteCRDs(ctx, ex); err != nil {
				return err
			}
			a.Info("Removed finalizers from DNSEntries and DNSProviders in shoot cluster", "namespace", namespace)
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

func (a *actuator) removeShootCustomResourcesFinalizersAndDeleteCRDs(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	shootClient, err := a.getShootClient(ctx, ex.Namespace)
	if err != nil {
		return err
	}

	if err := removeFinalizersFor(
		ctx,
		a.Logger,
		shootClient,
		"DNSEntries", "dnsentries.dns.gardener.cloud", ex.Namespace,
		&dnsv1alpha1.DNSEntryList{},
		func(list client.ObjectList) []objectWithDeepCopy[*dnsv1alpha1.DNSEntry] {
			l := list.(*dnsv1alpha1.DNSEntryList)
			result := make([]objectWithDeepCopy[*dnsv1alpha1.DNSEntry], len(l.Items))
			for i := range l.Items {
				result[i] = &l.Items[i]
			}
			return result
		}); err != nil {
		return err
	}

	if err := removeFinalizersFor(
		ctx,
		a.Logger,
		shootClient,
		"DNSProviders", "dnsproviders.dns.gardener.cloud", ex.Namespace,
		&dnsv1alpha1.DNSProviderList{},
		func(list client.ObjectList) []objectWithDeepCopy[*dnsv1alpha1.DNSProvider] {
			l := list.(*dnsv1alpha1.DNSProviderList)
			result := make([]objectWithDeepCopy[*dnsv1alpha1.DNSProvider], len(l.Items))
			for i := range l.Items {
				result[i] = &l.Items[i]
			}
			return result
		}); err != nil {
		return err
	}

	if err := a.deleteShootCustomResourceDefinitions(ctx, shootClient); err != nil {
		return fmt.Errorf("failed to delete DNS CRDs in shoot cluster: %w", err)
	}

	return nil
}

func (a *actuator) deleteShootCustomResourceDefinitions(ctx context.Context, shootClient client.Client) error {
	list := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := shootClient.List(ctx, list); err != nil {
		return err
	}

	for _, crd := range list.Items {
		if crd.Spec.Group == dnsv1alpha1.SchemeGroupVersion.Group {
			if err := kutil.DeleteObject(ctx, shootClient, &crd); err != nil {
				if k8serr.IsNotFound(err) {
					continue
				}
				return fmt.Errorf("failed to delete DNS CRD %q in shoot cluster: %w", crd.Name, err)
			}
			a.Info("Deleted DNS CRD in shoot cluster", "crd", crd.Name)
		}
	}
	return nil
}

func (a *actuator) getShootClient(ctx context.Context, namespace string) (client.Client, error) {
	_, shootClient, err := util.NewClientForShoot(ctx, a.Client(), namespace, client.Options{Scheme: a.Client().Scheme()}, extensionsconfigv1alpha1.RESTOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed creating client for shoot cluster: %w", err)
	}
	return shootClient, nil
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
		err = fmt.Errorf("waiting until shoot DNS entries have been deleted: %s", details)
		return &reconcilerutils.RequeueAfterError{
			Cause:        retry.RetriableError(util.DetermineError(err, helper.KnownCodes)),
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
		parts := strings.Split(k, "/")
		if len(parts) != 2 {
			status = append(status, fmt.Sprintf("unknown provider name: %s", k))
			continue
		}
		objectKey := client.ObjectKey{Namespace: parts[0], Name: parts[1]}
		provider := &dnsv1alpha1.DNSProvider{}
		if err := a.Client().Get(ctx, objectKey, provider); err != nil {
			status = append(status, fmt.Sprintf("error on retrieving status of provider %s: %s", k, err))
			continue
		}
		status = append(status, fmt.Sprintf("provider %s has status: %s", objectKey, ptr.Deref(provider.Status.Message, "unknown")))
	}
	return strings.Join(status, ", ")
}

// deleteDNSProviders deletes the external and additional providers
func (a *actuator) deleteDNSProviders(ctx context.Context, log logr.Logger, namespace string) error {
	dnsProviders := map[string]component.DeployWaiter{}

	if err := a.addCleanupOfOldAdditionalProviders(dnsProviders, ctx, log, namespace, false); err != nil {
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
	err = a.createOrUpdateSeedResources(helper.Context(), dnsconfig, cluster, helper.Extension(), false)
	if err != nil {
		return err
	}

	return helper.DeleteAll()
}

func (a *actuator) createOrUpdateShootResources(ctx context.Context, dnsconfig *apisservice.DNSConfig, cluster *controller.Cluster, namespace string) error {
	renderer, err := util.NewChartRendererForShoot(cluster.Shoot.Spec.Kubernetes.Version)
	if err != nil {
		return fmt.Errorf("could not create chart renderer: %w", err)
	}

	chartValues := map[string]any{
		"serviceName": service.ServiceName,
		"dnsProviderReplication": map[string]any{
			"enabled": a.replicateDNSProviders(dnsconfig),
		},
		"nextGeneration": map[string]any{
			"enabled": a.useNextGenerationController(dnsconfig),
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

func (a *actuator) createOrUpdateManagedResource(ctx context.Context, namespace, name, class string, renderer chartrenderer.Interface, chartName string, chartValues map[string]any, injectedLabels map[string]string) error {
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

func enableDNSProviderForShootDNSEntries(seedNamespace string) map[string]string {
	return map[string]string{DNSRealmAnnotation: fmt.Sprintf("%s,", seedNamespace)}
}

type objectWithDeepCopy[T client.Object] interface {
	client.Object
	DeepCopy() T
}

func removeFinalizersFor[T client.Object](
	ctx context.Context,
	log logr.Logger,
	shootClient client.Client,
	shortName, crdName, namespace string,
	list client.ObjectList,
	toObjectSlice func(list client.ObjectList) []objectWithDeepCopy[T]) error {
	if err := shootClient.Get(ctx, client.ObjectKey{Name: crdName}, &apiextensionsv1.CustomResourceDefinition{}); err != nil {
		if k8serr.IsNotFound(err) {
			log.Info("Skipping removal of finalizers from "+shortName+" in shoot cluster as CRD is not present", "namespace", namespace)
			return nil
		}
		return err
	}

	patchCount := 0
	// Remove finalizers from objects if managed by shoot-dns-service
	if err := shootClient.List(ctx, list); err != nil {
		return fmt.Errorf("failed to list %s in shoot cluster: %w", shortName, err)
	}
	for _, obj := range toObjectSlice(list) {
		if obj.GetAnnotations()[dns.CLASS_ANNOTATION] == "garden" {
			patch := client.MergeFrom(obj.DeepCopy())
			obj.SetFinalizers(nil)
			if err := client.IgnoreNotFound(shootClient.Patch(ctx, obj, patch)); err != nil {
				return fmt.Errorf("failed to remove finalizer from %s %q in shoot cluster: %w", shortName, client.ObjectKeyFromObject(obj), err)
			}
			log.Info("Removed finalizer from "+shortName+" in shoot cluster", "entry", client.ObjectKeyFromObject(obj), "namespace", namespace)
			patchCount++
		}
	}

	if patchCount == 0 {
		log.Info("No "+shortName+" found to patch in shoot cluster", "namespace", namespace)
	}

	return nil
}

func isAdditionalProvider(provider dnsv1alpha1.DNSProvider) bool {
	return provider.Labels[v1beta1constants.GardenRole] == DNSProviderRoleAdditional
}

func isReplicatedProvider(provider dnsv1alpha1.DNSProvider) bool {
	return provider.Labels[common.ShootDNSEntryLabelKey] != ""
}
