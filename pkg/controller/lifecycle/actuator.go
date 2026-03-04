// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	"github.com/gardener/gardener/extensions/pkg/util"
	gardencore "github.com/gardener/gardener/pkg/apis/core"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/controllerutils"
	reconcilerutils "github.com/gardener/gardener/pkg/controllerutils/reconciler"
	"github.com/gardener/gardener/pkg/extensions"
	"github.com/gardener/gardener/pkg/utils/chart"
	"github.com/gardener/gardener/pkg/utils/flow"
	gutil "github.com/gardener/gardener/pkg/utils/gardener"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
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

	"github.com/gardener/gardener-extension-shoot-dns-service/imagevector"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/helper"
	apisservice "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/validation"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/common"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

const (
	// SeedResourcesName is the name for resource describing the resources applied to the seed cluster.
	SeedResourcesName = service.ExtensionServiceName + "-seed"
	// ShootResourcesName is the name for resource describing the resources applied to the shoot cluster.
	ShootResourcesName = service.ExtensionServiceName + "-shoot"
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
	// ShootDNSServiceUseNextGenerationController is the label key for marking a seed to use the next generation DNS controller.
	// The label values "true" or "false" specify the default value, if not specified otherwise in the DNSConfig with the field `useNextGenerationController`.
	// The values "force-true" and "force-false" can be used to override the DNSConfig setting for all shoots in the seed.
	ShootDNSServiceUseNextGenerationController = "service.dns.extensions.gardener.cloud/use-next-generation-controller"

	// NextGenerationTargetClass is the target class for the next generation DNS controller.
	NextGenerationTargetClass = "gardendns-next-gen"
)

type controllerMode int

const (
	// controllerModeNormal is the normal operating mode of the shoot-dns-service controller manager: all controller are enabled or scaled down if hibernated.
	controllerModeNormal controllerMode = iota
	// controllerModeCleaningUp is the mode where the shoot-dns-service controller manager is cleaning up DNS entries in the control plane: only control plane controllers are enabled.
	controllerModeCleaningUp
	// controllerModeScaledDown is the mode where the shoot-dns-service controller manager is scaled down, e.g. during hibernation.
	controllerModeScaledDown
)

type extensionContext struct {
	ctx       context.Context
	log       logr.Logger
	ex        *extensionsv1alpha1.Extension
	dnsconfig *apisservice.DNSConfig
	cluster   *controller.Cluster
}

func (exCtx *extensionContext) useNextGenerationController() bool {
	value := ""
	if exCtx.cluster != nil && exCtx.cluster.Seed != nil {
		value = exCtx.cluster.Seed.Labels[ShootDNSServiceUseNextGenerationController]
	}
	switch value {
	case "force-true":
		return true
	case "force-false":
		return false
	case "true":
		return ptr.Deref(exCtx.dnsconfig.UseNextGenerationController, true)
	default:
		return ptr.Deref(exCtx.dnsconfig.UseNextGenerationController, false)
	}
}

// NewActuator returns an actuator responsible for Extension resources.
func NewActuator(c client.Client, scheme *runtime.Scheme, chartRenderer chartrenderer.Interface, config config.DNSServiceConfig,
	managedResourcesAccess managedResourcesAccess,
	shootClientAccess shootClientAccess,
	newProviderDeployWaiterFactory *newProviderDeployWaiterFactory,
	fastTestMode bool,
) extension.Actuator {
	return &actuator{
		client:                         c,
		config:                         config,
		renderer:                       chartRenderer,
		decoder:                        serializer.NewCodecFactory(scheme, serializer.EnableStrict).UniversalDecoder(),
		managedResourceAccess:          managedResourcesAccess,
		shootClientAccess:              shootClientAccess,
		newProviderDeployWaiterFactory: newProviderDeployWaiterFactory,
		fastTestMode:                   fastTestMode,
	}
}

type actuator struct {
	config   config.DNSServiceConfig
	client   client.Client
	renderer chartrenderer.Interface
	decoder  runtime.Decoder

	managedResourceAccess          managedResourcesAccess
	shootClientAccess              shootClientAccess
	newProviderDeployWaiterFactory *newProviderDeployWaiterFactory
	fastTestMode                   bool
}

// Reconcile the Extension resource.
func (a *actuator) Reconcile(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	exCtx, err := a.prepareExtensionContext(ctx, log, ex)
	if err != nil {
		return err
	}

	// Shoots that don't specify a DNS domain don't get a DNS service
	if exCtx.cluster.Shoot.Spec.DNS == nil {
		log.Info("DNS domain is not specified, therefore no shoot dns service is installed", "shoot", ex.Namespace)
		return a.Delete(ctx, log, ex)
	}

	if ex.Status.State != nil && common.IsRestoring(ex) {
		if err := a.ResurrectFrom(ctx, log, ex); err != nil {
			return err
		}
	}

	if err := a.createOrUpdateShootResources(exCtx); err != nil {
		return err
	}
	if err := a.createOrUpdateSeedResources(exCtx, controllerModeNormal); err != nil {
		return err
	}
	return a.createOrUpdateDNSProviders(exCtx)
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

func (a *actuator) ResurrectFrom(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	handler, err := common.NewStateHandler(ctx, log, a.client, ex)
	if err != nil {
		return err
	}

	log.Info("resurrect DNS entries", "namespace", ex.Namespace, "name", ex.Name)

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
		err := a.client.Create(ctx, obj)
		if err != nil && !k8serr.IsAlreadyExists(err) {
			lasterr = err
		}
	}

	return lasterr
}

// Delete the Extension resource.
func (a *actuator) Delete(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	exCtx, err := a.prepareExtensionContext(ctx, log, ex)
	if err != nil {
		return err
	}
	return a.delete(exCtx, false)
}

// ForceDelete the Extension resource.
func (a *actuator) ForceDelete(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	exCtx, err := a.prepareExtensionContext(ctx, log, ex)
	if err != nil {
		return err
	}

	// try to delete managed DNS entries normally first
	if err := a.deleteManagedDNSEntries(exCtx); err != nil {
		// ignore failed deletion of DNSEntries
		if _, ok := err.(*reconcilerutils.RequeueAfterError); !ok {
			return err
		}
	}

	if err := a.deleteSeedResources(exCtx, false, true); err != nil {
		return err
	}

	entriesHelper := common.NewShootDNSEntriesHelper(ctx, a.client, ex)
	if err := entriesHelper.ForceDeleteAll(); err != nil {
		return fmt.Errorf("force deletion of DNSEntries failed: %w", err)
	}

	if a.isManagingDNSProviders(exCtx.cluster.Shoot.Spec.DNS) {
		// no forced deletion of providers needed, as they can be deleted normally as soon as there are no DNSEntries anymore
		if err := a.deleteDNSProviders(exCtx); err != nil {
			return err
		}
		// seed resources may have been recreated by DNS provider deletion
		if err := a.deleteSeedResources(exCtx, false, true); err != nil {
			return err
		}
	}

	return nil
}

func (a *actuator) delete(exCtx extensionContext, migrate bool) error {
	if err := a.deleteSeedResources(exCtx, migrate, false); err != nil {
		return err
	}
	return a.deleteShootResources(exCtx.ctx, exCtx.ex.Namespace)
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
	exCtx, err := a.prepareExtensionContext(ctx, log, ex)
	if err != nil {
		return err
	}

	// Keep objects for shoot managed resources so that they are not deleted from the shoot during the migration
	if err := a.managedResourceAccess.SetKeepObjects(ctx, ex.GetNamespace(), ShootResourcesName, true); err != nil {
		return err
	}

	if err := a.ignoreDNSEntriesForMigration(ctx, ex); err != nil {
		return err
	}

	if ex.Annotations[DropDNSEntriesStateOnMigration] == "true" {
		if err := a.ensureStateDropped(exCtx); err != nil {
			return err
		}
	} else {
		if err := a.ensureStateRefreshed(exCtx); err != nil {
			return err
		}
	}

	return a.delete(exCtx, true)
}

func (a *actuator) prepareExtensionContext(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) (extensionContext, error) {
	cluster, err := controller.GetCluster(ctx, a.client, ex.Namespace)
	if err != nil {
		return extensionContext{}, err
	}

	dnsConfig, err := a.extractDNSConfig(ex)
	if err != nil {
		return extensionContext{}, err
	}

	return extensionContext{
		ctx:       ctx,
		log:       log,
		ex:        ex,
		cluster:   cluster,
		dnsconfig: dnsConfig,
	}, nil
}

func (a *actuator) ignoreDNSEntriesForMigration(ctx context.Context, ex *extensionsv1alpha1.Extension) error {
	entriesHelper := common.NewShootDNSEntriesHelper(ctx, a.client, ex)
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
		if err := client.IgnoreNotFound(a.client.Patch(ctx, &entry, patch)); err != nil {
			return fmt.Errorf("failed to ignore DNS entry %q: %w", entry.Name, err)
		}
	}
	return nil
}

func (a *actuator) waitForEntryReconciliation(ctx context.Context, log logr.Logger, ex *extensionsv1alpha1.Extension) error {
	entriesHelper := common.NewShootDNSEntriesHelper(ctx, a.client, ex)
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
		if err := client.IgnoreNotFound(a.client.Patch(ctx, &entry, patch)); err != nil {
			return fmt.Errorf("failed to revert ignore DNS entry %q: %w", entry.Name, err)
		}
	}

	// wait for all entries to be reconciled i.e., gardener.cloud/operation annotation is removed
	start := time.Now()
	for _, entry := range list {
		for {
			if err := a.client.Get(ctx, client.ObjectKeyFromObject(&entry), &entry); err != nil {
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
	return a.config.ManageDNSProviders && dns != nil && dns.Domain != nil
}

func (a *actuator) isHibernated(cluster *controller.Cluster) bool {
	hibernation := cluster.Shoot.Spec.Hibernation
	return hibernation != nil && hibernation.Enabled != nil && *hibernation.Enabled
}

func (a *actuator) createOrUpdateSeedResources(exCtx extensionContext, mode controllerMode) error {
	var err error
	namespace := exCtx.ex.Namespace

	exCtx.log.Info("Creating/updating seed resources", "namespace", namespace)
	if !common.IsRestoring(exCtx.ex) && !common.IsMigrating(exCtx.ex) {
		if err := a.ensureStateDropped(exCtx); err != nil {
			return err
		}
	}

	shootID, creatorLabelValue, err := common.ShootID(exCtx.cluster)
	if err != nil {
		return err
	}

	seedID := a.config.SeedID
	if seedID == "" {
		if exCtx.cluster.Seed.Status.ClusterIdentity == nil {
			return fmt.Errorf("missing 'seed.status.clusterIdentity' in cluster")
		}
		seedID = *exCtx.cluster.Seed.Status.ClusterIdentity
		a.config.SeedID = seedID
	}

	replicas := 1
	switch mode {
	case controllerModeNormal:
		if a.isHibernated(exCtx.cluster) {
			replicas = 0
		}
	case controllerModeCleaningUp:
		if !exCtx.useNextGenerationController() {
			replicas = 0
		}
	case controllerModeScaledDown:
		replicas = 0
	}

	chartValues := map[string]any{
		"serviceName":                      service.ServiceName,
		"genericTokenKubeconfigSecretName": extensions.GenericTokenKubeconfigSecretNameFromCluster(exCtx.cluster),
		"replicas":                         replicas,
		"creatorLabelValue":                creatorLabelValue,
		"shootId":                          shootID,
		"seedId":                           seedID,
		"dnsClass":                         a.config.DNSClass,
		"dnsProviderReplication": map[string]any{
			"enabled": a.replicateDNSProviders(exCtx.dnsconfig),
		},
		"nextGeneration": map[string]any{
			"enabled":                           exCtx.useNextGenerationController(),
			"dnsClass":                          NextGenerationTargetClass,
			"restrictToControlPlaneControllers": mode == controllerModeCleaningUp,
		},
	}
	if exCtx.useNextGenerationController() {
		stringsList := []string{}
		for _, re := range a.config.InternalGCPWorkloadIdentityConfig.AllowedServiceAccountImpersonationURLRegExps {
			stringsList = append(stringsList, re.String())
		}
		chartValues["workloadIdentity"] = map[string]any{
			"gcp": map[string]any{
				"allowedTokenURLs": a.config.InternalGCPWorkloadIdentityConfig.AllowedTokenURLs,
				"allowedServiceAccountImpersonationURLRegExps": stringsList,
			},
		}
	}

	if err := gutil.NewShootAccessSecret(service.ShootAccessSecretName, namespace).Reconcile(exCtx.ctx, a.client); err != nil {
		return err
	}
	chartValues["targetClusterSecret"] = gutil.SecretNamePrefixShootAccess + service.ShootAccessSecretName

	chartValues, err = chart.InjectImages(chartValues, imagevector.ImageVector(), []string{service.ImageName, service.ImageNameNextGeneration})
	if err != nil {
		return fmt.Errorf("failed to find image version for %s and %s: %v", service.ImageName, service.ImageNameNextGeneration, err)
	}

	exCtx.log.Info("Component is being applied", "component", service.ExtensionServiceName, "namespace", namespace)
	return a.managedResourceAccess.CreateOrUpdate(exCtx.ctx, namespace, SeedResourcesName, "seed", a.renderer, service.SeedChartName, chartValues, nil)
}

func (a *actuator) ensureStateDropped(exCtx extensionContext) error {
	// The DNSEntries are not stored in the extension state, as they are only needed for control plane migration during the
	// restore step.
	handler, err := common.NewStateHandler(exCtx.ctx, exCtx.log, a.client, exCtx.ex)
	if err != nil {
		exCtx.log.Info("ignoring state handler error", "error", err, "namespace", exCtx.ex.Namespace)
	}
	handler.DropAllEntries()
	return handler.Update("cleanup")
}

func (a *actuator) ensureStateRefreshed(exCtx extensionContext) error {
	// The DNSEntries in the control plane are listed and stored in the extension state for migration.
	handler, err := common.NewStateHandler(exCtx.ctx, exCtx.log, a.client, exCtx.ex)
	if err != nil {
		exCtx.log.Info("ignoring state handler error", "error", err, "namespace", exCtx.ex.Namespace)
	}
	exCtx.log.Info("refreshing state", "err", err, "namespace", exCtx.ex.Namespace, "name", exCtx.ex.Name)
	if _, err = handler.Refresh(); err != nil {
		exCtx.log.Info("refreshing state failed", "err", err, "namespace", exCtx.ex.Namespace, "name", exCtx.ex.Name)
		return err
	}
	return handler.Update("refresh")
}

func (a *actuator) createOrUpdateDNSProviders(exCtx extensionContext) error {
	if !a.isManagingDNSProviders(exCtx.cluster.Shoot.Spec.DNS) {
		return nil
	}

	var err, result error
	namespace := exCtx.ex.Namespace
	deployers := map[string]component.DeployWaiter{}

	hibernated := a.isHibernated(exCtx.cluster)
	if !hibernated {
		external, err := a.prepareDefaultExternalDNSProvider(exCtx)
		if err != nil {
			return err
		}

		resources := exCtx.cluster.Shoot.Spec.Resources
		providers := map[string]*dnsv1alpha1.DNSProvider{}
		providers[ExternalDNSProviderName] = nil // remember for deletion
		if external != nil {
			providers[ExternalDNSProviderName] = buildDNSProvider(external, namespace, ExternalDNSProviderName, "")
		}

		result = a.addAdditionalDNSProviders(providers, exCtx, result, resources)

		for name, p := range providers {
			var dw component.DeployWaiter
			if p != nil {
				dw = a.newProviderDeployWaiterFactory.New(exCtx, p)
			}
			deployers[name] = dw
		}
	} else {
		if err := a.deleteManagedDNSEntries(exCtx); err != nil {
			return err
		}
	}

	if err := a.addCleanupOfOldAdditionalProviders(deployers, exCtx, !hibernated); err != nil {
		result = multierror.Append(result, err)
	}

	err = a.deployDNSProviders(exCtx.ctx, deployers)
	if err != nil {
		result = multierror.Append(result, err)
	}

	if result != nil {
		return result
	}

	if !hibernated {
		return nil
	}

	return a.prepareSeedResources(exCtx, controllerModeScaledDown)
}

// addCleanupOfOldAdditionalProviders adds destroy DeployWaiter to clean up old orphaned additional providers
func (a *actuator) addCleanupOfOldAdditionalProviders(dnsProviders map[string]component.DeployWaiter, exCtx extensionContext, keepReplicatedProviders bool) error {
	namespace := exCtx.ex.Namespace
	providerList := &dnsv1alpha1.DNSProviderList{}
	if err := a.client.List(
		exCtx.ctx,
		providerList,
		client.InNamespace(namespace),
	); err != nil {
		return err
	}

	count := 0
	for _, provider := range providerList.Items {
		if !isAdditionalProvider(provider) && (keepReplicatedProviders || !isReplicatedProvider(provider)) {
			continue
		}
		if _, ok := dnsProviders[provider.Name]; !ok {
			p := provider
			dnsProviders[provider.Name] = component.OpDestroyAndWait(a.newProviderDeployWaiterFactory.New(exCtx, &p))
			count++
		}
	}

	if dw, ok := dnsProviders[ExternalDNSProviderName]; dw == nil && ok {
		// delete non-migrated non-default external DNS provider if it exists
		provider := &dnsv1alpha1.DNSProvider{}
		if err := a.client.Get(
			exCtx.ctx,
			client.ObjectKey{Namespace: namespace, Name: ExternalDNSProviderName},
			provider,
		); err == nil {
			dnsProviders[provider.Name] = component.OpDestroyAndWait(a.newProviderDeployWaiterFactory.New(exCtx, provider))
			count++
		}
	}

	if count > 0 {
		if err := a.prepareSeedResources(exCtx, controllerModeCleaningUp); err != nil {
			return err
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

func (a *actuator) addAdditionalDNSProviders(providers map[string]*dnsv1alpha1.DNSProvider, exCtx extensionContext, result error, resources []gardencorev1beta1.NamedResourceReference) error {
	namespace := exCtx.ex.Namespace
	for i, provider := range exCtx.dnsconfig.Providers {
		p := provider

		providerType := p.Type
		if providerType == nil {
			result = multierror.Append(result, fmt.Errorf("dns provider[%d] doesn't specify a type", i))
			continue
		}

		if *providerType == gardencore.DNSUnmanaged {
			exCtx.log.Info(fmt.Sprintf("Skipping deployment of DNS provider[%d] since it specifies type %q", i, gardencore.DNSUnmanaged))
			continue
		}

		resourceName := oneOf(p.SecretName, p.Credentials)
		mappedSecretName, err := lookupReference(resources, resourceName, i)
		if err != nil {
			result = multierror.Append(result, err)
			continue
		}

		providerName := fmt.Sprintf("%s-%s", *providerType, resourceName)
		providers[providerName] = nil

		secret := &corev1.Secret{}
		if err := a.client.Get(
			exCtx.ctx,
			client.ObjectKey{Namespace: namespace, Name: mappedSecretName},
			secret,
		); err != nil {
			result = multierror.Append(result, fmt.Errorf("could not get dns provider[%d] secret %q -> %q: %w", i, resourceName, mappedSecretName, err))
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
	secretName := oneOf(p.SecretName, p.Credentials)
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

func lookupReference(resources []gardencorev1beta1.NamedResourceReference, resourceName string, index int) (string, error) {
	if resourceName == "" {
		return "", fmt.Errorf("dns provider[%d] doesn't specify a secretName or credentials field", index)
	}

	for _, res := range resources {
		if res.Name == resourceName {
			if res.ResourceRef.Kind == "WorkloadIdentity" {
				return v1beta1constants.ReferencedWorkloadIdentityPrefix + res.ResourceRef.Name, nil
			}
			return v1beta1constants.ReferencedResourcesPrefix + res.ResourceRef.Name, nil
		}
	}

	return "", fmt.Errorf("dns provider[%d] secretName/credentials %s not found in referenced resources", index, resourceName)
}

func (a *actuator) prepareDefaultExternalDNSProvider(exCtx extensionContext) (*apisservice.DNSProvider, error) {
	for _, provider := range exCtx.cluster.Shoot.Spec.DNS.Providers {
		if provider.Primary != nil && *provider.Primary {
			return nil, nil
		}
	}

	if a.useRemoteDefaultDomain(exCtx) {
		secretName, err := a.copyRemoteDefaultDomainSecret(exCtx.ctx, exCtx.ex.Namespace)
		if err != nil {
			return nil, err
		}
		remoteType := "remote"
		return &apisservice.DNSProvider{
			Domains: &apisservice.DNSIncludeExclude{
				Include: []string{*exCtx.cluster.Shoot.Spec.DNS.Domain},
				Exclude: []string{"api." + *exCtx.cluster.Shoot.Spec.DNS.Domain}, // exclude external kube-apiserver domain
			},
			SecretName: &secretName,
			Type:       &remoteType,
		}, nil
	}

	secretRef, providerType, zone, err := GetSecretRefFromDNSRecordExternal(exCtx.ctx, a.client, exCtx.ex.Namespace, exCtx.cluster.Shoot.Name)
	if err != nil || secretRef == nil {
		return nil, err
	}
	provider := &apisservice.DNSProvider{
		Domains: &apisservice.DNSIncludeExclude{
			Include: []string{*exCtx.cluster.Shoot.Spec.DNS.Domain},
			Exclude: []string{"api." + *exCtx.cluster.Shoot.Spec.DNS.Domain}, // exclude external kube-apiserver domain
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

func (a *actuator) useRemoteDefaultDomain(exCtx extensionContext) bool {
	if exCtx.useNextGenerationController() {
		// The next generation controller does not support remote default domain handling
		return false
	}
	if a.config.RemoteDefaultDomainSecret != nil && exCtx.cluster.Seed.Labels != nil {
		annot, ok := exCtx.cluster.Seed.Labels[ShootDNSServiceUseRemoteDefaultDomainLabel]
		return ok && annot == "true"
	}
	return false
}

func (a *actuator) copyRemoteDefaultDomainSecret(ctx context.Context, namespace string) (string, error) {
	secretOrg := &corev1.Secret{}
	err := a.client.Get(ctx, *a.config.RemoteDefaultDomainSecret, secretOrg)
	if err != nil {
		return "", err
	}

	secret := &corev1.Secret{}
	secret.Namespace = namespace
	secret.Name = "shoot-dns-service-remote-default-domains"
	_, err = controllerutils.CreateOrGetAndMergePatch(ctx, a.client, secret, func() error {
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
	return a.config.ReplicateDNSProviders
}

func (a *actuator) deleteSeedResources(exCtx extensionContext, migrate, force bool) error {
	namespace := exCtx.ex.Namespace
	exCtx.log.Info("Component is being deleted", "component", service.ExtensionServiceName, "namespace", namespace)

	// DNSEntries and DNSProvider are deleted after the seed resources have been deleted, so that the
	// shoot-dns-service deployment is already gone and cannot resurrect resources from the shoot cluster.
	if !force {
		if !migrate {
			if err := a.deleteManagedDNSEntries(exCtx); err != nil {
				return err
			}
			// need to remove finalizers from DNSEntries and DNSProviders on shoot as
			// shoot-dns-service is not running anymore on the seed
			if err := a.removeShootCustomResourcesFinalizersAndDeleteCRDs(exCtx); err != nil {
				return err
			}
			exCtx.log.Info("Removed finalizers from DNSEntries and DNSProviders in shoot cluster", "namespace", namespace)
		}

		if a.isManagingDNSProviders(exCtx.cluster.Shoot.Spec.DNS) {
			if err := a.deleteDNSProviders(exCtx); err != nil {
				return err
			}
		}
	}

	if err := a.managedResourceAccess.Delete(exCtx.ctx, namespace, SeedResourcesName); err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(exCtx.ctx, 2*time.Minute)
	defer cancel()
	if err := a.managedResourceAccess.WaitUntilDeleted(timeoutCtx, namespace, SeedResourcesName); err != nil {
		return err
	}

	return kutil.DeleteObject(exCtx.ctx, a.client, &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: gutil.SecretNamePrefixShootAccess + service.ShootAccessSecretName, Namespace: namespace}})
}

func (a *actuator) removeShootCustomResourcesFinalizersAndDeleteCRDs(exCtx extensionContext) error {
	shootClient, err := a.shootClientAccess.GetShootClient(exCtx.ctx, exCtx.ex.Namespace)
	if err != nil {
		return err
	}

	if err := removeFinalizersFor(
		exCtx.ctx,
		exCtx.log,
		shootClient,
		"DNSEntries", "dnsentries.dns.gardener.cloud", exCtx.ex.Namespace,
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
		exCtx.ctx,
		exCtx.log,
		shootClient,
		"DNSProviders", "dnsproviders.dns.gardener.cloud", exCtx.ex.Namespace,
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

	if err := a.deleteShootCustomResourceDefinitions(exCtx.ctx, exCtx.log, shootClient); err != nil {
		return fmt.Errorf("failed to delete DNS CRDs in shoot cluster: %w", err)
	}

	return nil
}

func (a *actuator) deleteShootCustomResourceDefinitions(ctx context.Context, log logr.Logger, shootClient client.Client) error {
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
			log.Info("Deleted DNS CRD in shoot cluster", "crd", crd.Name)
		}
	}
	return nil
}

func (a *actuator) deleteManagedDNSEntries(exCtx extensionContext) error {
	entriesHelper := common.NewShootDNSEntriesHelper(exCtx.ctx, a.client, exCtx.ex)
	list, err := entriesHelper.List()
	if err != nil {
		return err
	}
	if len(list) > 0 {
		// need to wait until all shoot DNS entries have been deleted
		// for robustness scale deployment of shoot-dns-service-seed down to 0
		// and delete all shoot DNS entries
		if err := a.prepareSeedResources(exCtx, controllerModeCleaningUp); err != nil {
			return fmt.Errorf("preparing shoot-dns-service deployment for cleanup failed: %w", err)
		}
		if err := entriesHelper.DeleteAll(); err != nil {
			return fmt.Errorf("deleting all DNSEntries in control plane failed: %w", err)
		}
		exCtx.log.Info("Waiting until all shoot DNS entries have been deleted", "component", service.ExtensionServiceName, "namespace", exCtx.ex.Namespace)
		for range 7 {
			waitTime := 5 * time.Second
			if a.fastTestMode {
				waitTime = 20 * time.Millisecond
			}
			time.Sleep(waitTime)
			list, err = entriesHelper.List()
			if err != nil || len(list) == 0 {
				if err != nil {
					exCtx.log.Info("listing DNS entries failed", "error", err, "namespace", exCtx.ex.Namespace)
				}
				break
			}
		}
		if len(list) > 0 {
			details := a.collectProviderDetailsOnDeletingDNSEntries(exCtx.ctx, list)
			err = fmt.Errorf("waiting until shoot DNS entries have been deleted: %s", details)
			return &reconcilerutils.RequeueAfterError{
				Cause:        retry.RetriableError(util.DetermineError(err, helper.KnownCodes)),
				RequeueAfter: 15 * time.Second,
			}
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
		if err := a.client.Get(ctx, objectKey, provider); err != nil {
			status = append(status, fmt.Sprintf("error on retrieving status of provider %s: %s", k, err))
			continue
		}
		status = append(status, fmt.Sprintf("provider %s has status: %s", objectKey, ptr.Deref(provider.Status.Message, "unknown")))
	}
	return strings.Join(status, ", ")
}

// deleteDNSProviders deletes the external and additional providers
func (a *actuator) deleteDNSProviders(exCtx extensionContext) error {
	dnsProviders := map[string]component.DeployWaiter{}

	if err := a.addCleanupOfOldAdditionalProviders(dnsProviders, exCtx, false); err != nil {
		return err
	}

	return a.deployDNSProviders(exCtx.ctx, dnsProviders)
}

func (a *actuator) prepareSeedResources(exCtx extensionContext, mode controllerMode) error {
	return a.createOrUpdateSeedResources(exCtx, mode)
}

func (a *actuator) createOrUpdateShootResources(exCtx extensionContext) error {
	renderer, err := util.NewChartRendererForShoot(exCtx.cluster.Shoot.Spec.Kubernetes.Version)
	if err != nil {
		return fmt.Errorf("could not create chart renderer: %w", err)
	}

	chartValues := map[string]any{
		"serviceName": service.ServiceName,
		"dnsProviderReplication": map[string]any{
			"enabled": a.replicateDNSProviders(exCtx.dnsconfig),
		},
		"nextGeneration": map[string]any{
			"enabled": exCtx.useNextGenerationController(),
		},
		"shootAccessServiceAccountName": service.ShootAccessServiceAccountName,
	}
	injectedLabels := map[string]string{v1beta1constants.ShootNoCleanup: "true"}

	return a.managedResourceAccess.CreateOrUpdate(exCtx.ctx, exCtx.ex.Namespace, ShootResourcesName, "", renderer, service.ShootChartName, chartValues, injectedLabels)
}

func (a *actuator) deleteShootResources(ctx context.Context, namespace string) error {
	if err := a.managedResourceAccess.Delete(ctx, namespace, ShootResourcesName); err != nil {
		return err
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return a.managedResourceAccess.WaitUntilDeleted(timeoutCtx, namespace, ShootResourcesName)
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

func oneOf(strs ...*string) string {
	for _, s := range strs {
		if s != nil && *s != "" {
			return *s
		}
	}
	return ""
}
