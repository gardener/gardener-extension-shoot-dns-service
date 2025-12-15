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
	dnsapisconfig "github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
	dnsapp "github.com/gardener/external-dns-management/pkg/dnsman2/app"
	extensionscontroller "github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/extensions/pkg/controller/extension"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/chartrenderer"
	"github.com/gardener/gardener/pkg/logger"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/yaml"

	apisservice "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service"
	servicev1alpha1 "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/service/v1alpha1"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
)

type testShootClientAccess struct {
	shootClient       client.Client
	expectedNamespace string
}

var _ shootClientAccess = &testShootClientAccess{}

func (a *testShootClientAccess) GetShootClient(_ context.Context, namespace string) (client.Client, error) {
	if a.expectedNamespace != namespace {
		return nil, fmt.Errorf("unexpected namespace: got %s, want %s", namespace, a.expectedNamespace)
	}
	return a.shootClient, nil
}

type managedResourceCall string

const (
	managedResourceCallCreateOrUpdate   managedResourceCall = "CreateOrUpdate"
	managedResourceCallDelete           managedResourceCall = "Delete"
	managedResourceCallWaitUntilDeleted managedResourceCall = "WaitUntilDeleted"
	managedResourceCallSetKeepObjects                       = "SetKeepObjects"
)

type mockManagedResource struct {
	namespace      string
	name           string
	class          string
	chartName      string
	values         map[string]any
	injectedLabels map[string]string

	keepObjects bool
}

type expectedManagedResource struct {
	call       managedResourceCall
	namespace  string
	name       string
	checkFuncs []func(mmr *mockManagedResource)
}

type testManagedResourcesAccess struct {
	log            logr.Logger
	expectedCalls  []expectedManagedResource
	callsProcessed int
}

func newTestManagedResourcesAccess() *testManagedResourcesAccess {
	return &testManagedResourcesAccess{
		log: logf.Log.WithName("testManagedResourcesAccess"),
	}
}

var _ managedResourcesAccess = &testManagedResourcesAccess{}

func (t *testManagedResourcesAccess) CreateOrUpdate(_ context.Context, namespace, name, class string, _ chartrenderer.Interface, chartName string, chartValues map[string]any, injectedLabels map[string]string) error {
	expected := t.checkCall(managedResourceCallCreateOrUpdate, namespace, name)
	mmr := &mockManagedResource{
		namespace:      namespace,
		name:           name,
		class:          class,
		chartName:      chartName,
		values:         chartValues,
		injectedLabels: injectedLabels,
	}
	for _, checkFunc := range expected.checkFuncs {
		checkFunc(mmr)
	}
	return nil
}

func (t *testManagedResourcesAccess) Delete(_ context.Context, namespace string, name string) error {
	expected := t.checkCall(managedResourceCallDelete, namespace, name)
	mmr := &mockManagedResource{
		namespace: namespace,
		name:      name,
	}
	for _, checkFunc := range expected.checkFuncs {
		checkFunc(mmr)
	}
	return nil
}

func (t *testManagedResourcesAccess) WaitUntilDeleted(_ context.Context, namespace, name string) error {
	expected := t.checkCall(managedResourceCallWaitUntilDeleted, namespace, name)
	mmr := &mockManagedResource{
		namespace: namespace,
		name:      name,
	}
	for _, checkFunc := range expected.checkFuncs {
		checkFunc(mmr)
	}
	return nil
}

func (t *testManagedResourcesAccess) checkCall(call managedResourceCall, namespace, name string) expectedManagedResource {
	t.log.Info(string(call), "call", t.callsProcessed)
	if len(t.expectedCalls) == 0 {
		Fail(fmt.Sprintf("unexpected call of ManagedResource: %s %s/%s", call, namespace, name))
	}
	next := t.expectedCalls[0]
	if next.call != call || next.namespace != namespace || next.name != name {
		Fail(fmt.Sprintf("unexpected call of ManagedResource: got %s %s/%s, want %s %s/%s", call, namespace, name, next.call, next.namespace, next.name))
	}
	t.expectedCalls = t.expectedCalls[1:]
	t.callsProcessed++
	return next
}

func (t *testManagedResourcesAccess) SetKeepObjects(_ context.Context, namespace, name string, keepObjects bool) error {
	expected := t.checkCall(managedResourceCallSetKeepObjects, namespace, name)
	mmr := &mockManagedResource{
		namespace:   namespace,
		name:        name,
		keepObjects: keepObjects,
	}
	for _, checkFunc := range expected.checkFuncs {
		checkFunc(mmr)
	}
	return nil
}

func (t *testManagedResourcesAccess) ExpectCall(call managedResourceCall, namespace, name string, checkFuncs ...func(mmr *mockManagedResource)) {
	t.expectedCalls = append(t.expectedCalls, expectedManagedResource{
		call:       call,
		namespace:  namespace,
		name:       name,
		checkFuncs: checkFuncs,
	})
}

func (t *testManagedResourcesAccess) ExpectCreateOrUpdate(namespace, name string, checkFuncs ...func(mmr *mockManagedResource)) {
	t.ExpectCall(managedResourceCallCreateOrUpdate, namespace, name, checkFuncs...)
}

func (t *testManagedResourcesAccess) ExpectDelete(namespace, name string, checkFuncs ...func(mmr *mockManagedResource)) {
	t.ExpectCall(managedResourceCallDelete, namespace, name, checkFuncs...)
}

func (t *testManagedResourcesAccess) ExpectWaitUntilDeleted(namespace, name string, checkFuncs ...func(mmr *mockManagedResource)) {
	t.ExpectCall(managedResourceCallWaitUntilDeleted, namespace, name, checkFuncs...)
}

func (t *testManagedResourcesAccess) ConsumedAllExpectations() {
	if len(t.expectedCalls) != 0 {
		var remaining []string
		for _, e := range t.expectedCalls {
			remaining = append(remaining, fmt.Sprintf("%s %s/%s", e.call, e.namespace, e.name))
		}
		Fail(fmt.Sprintf("not all expected CreateOrUpdate calls were made, remaining: %s", strings.Join(remaining, ", ")))
	}
}

type dnsControllerMock struct {
	ctx                 context.Context
	client              client.Client
	expectedProviders   []*dnsv1alpha1.DNSProvider
	expectedTargetClass string

	providersReady sets.Set[client.ObjectKey]
}

func newDNSControllerMock(ctx context.Context, c client.Client, expectedTargetClass string, expectedProviders ...*dnsv1alpha1.DNSProvider) *dnsControllerMock {
	return &dnsControllerMock{
		ctx:                 ctx,
		client:              c,
		expectedTargetClass: expectedTargetClass,
		expectedProviders:   expectedProviders,
		providersReady:      sets.New[client.ObjectKey](),
	}
}

func (d *dnsControllerMock) run() {
	defer GinkgoRecover()

	ticker := time.Tick(20 * time.Millisecond)
	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker:
			for _, expected := range d.expectedProviders {
				key := client.ObjectKey{Namespace: expected.Namespace, Name: expected.Name}
				actual := &dnsv1alpha1.DNSProvider{}
				err := d.client.Get(d.ctx, key, actual)
				if err != nil {
					continue
				}
				Expect(actual.Spec).To(Equal(expected.Spec))
				Expect(actual.Annotations["dns.gardener.cloud/class"]).To(Equal(d.expectedTargetClass))
				if actual.Status.State != "Ready" {
					d.providersReady.Insert(key)
					actual.Status.State = "Ready"
					Expect(d.client.Status().Update(d.ctx, actual)).To(Succeed(), "failed to update DNSProvider status to Ready")
				}
			}
		}
	}
}

var _ = Describe("Actuator", func() {
	const (
		providerSecretName = "provider-secret"
	)

	var (
		ctx                    context.Context
		log                    logr.Logger
		scheme                 *runtime.Scheme
		decoder                runtime.Decoder
		seedClient             client.Client
		shootClient            client.Client
		dnsServiceConfig       config.DNSServiceConfig
		managedResourcesAccess *testManagedResourcesAccess
		actuator               extension.Actuator
		ex                     *extensionsv1alpha1.Extension
		cluster                *extensionsv1alpha1.Cluster
		providerExternal       *dnsv1alpha1.DNSProvider
		providerAdditional     *dnsv1alpha1.DNSProvider

		prepareControlPlane = func(useNextGenerationController bool) {
			GinkgoHelper()
			Expect(seedClient.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "shoot--foo--bar"}})).To(Succeed(), "failed to create shoot namespace")

			cluster.Spec.Shoot.Object.(*gardencorev1beta1.Shoot).Spec.Extensions[0].ProviderConfig.Object.(*servicev1alpha1.DNSConfig).UseNextGenerationController = ptr.To(useNextGenerationController)
			Expect(seedClient.Create(ctx, cluster)).To(Succeed(), "failed to create cluster resource")

			fetchedCluster := &extensionsv1alpha1.Cluster{}
			Expect(seedClient.Get(ctx, client.ObjectKeyFromObject(cluster), fetchedCluster)).To(Succeed(), "failed to get cluster resource")
			shoot := &gardencorev1beta1.Shoot{}
			_, _, err := decoder.Decode(fetchedCluster.Spec.Shoot.Raw, nil, shoot)
			Expect(err).ToNot(HaveOccurred(), "failed to decode shoot from cluster resource")
			ex = &extensionsv1alpha1.Extension{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "shoot-dns-service",
					Namespace: cluster.Name,
				},
				Spec: extensionsv1alpha1.ExtensionSpec{
					DefaultSpec: extensionsv1alpha1.DefaultSpec{
						ProviderConfig: shoot.Spec.Extensions[0].ProviderConfig,
						Type:           "shoot-dns-service",
					},
				},
			}
			Expect(seedClient.Create(ctx, ex)).To(Succeed(), "failed to create extension resource")

			dnsExternal := &extensionsv1alpha1.DNSRecord{
				ObjectMeta: metav1.ObjectMeta{
					Name:      shoot.Name + "-external",
					Namespace: cluster.Name,
				},
				Spec: extensionsv1alpha1.DNSRecordSpec{
					DefaultSpec: extensionsv1alpha1.DefaultSpec{
						Type: "google-clouddns",
					},
					SecretRef: corev1.SecretReference{
						Name:      "external-dns-secret",
						Namespace: cluster.Name,
					},
					Zone: ptr.To("zone-12345"),
				},
			}
			Expect(seedClient.Create(ctx, dnsExternal)).To(Succeed(), "failed to create dnsrecord external resource")

			providerSecret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ref-original-" + providerSecretName,
					Namespace: cluster.Name,
				},
				Data: map[string][]byte{
					"key": []byte("value"),
				},
			}
			Expect(seedClient.Create(ctx, providerSecret)).To(Succeed(), "failed to create fake referenced provider secret")

			actuator = NewActuator(seedClient, scheme, nil, dnsServiceConfig,
				managedResourcesAccess,
				&testShootClientAccess{shootClient: shootClient, expectedNamespace: "shoot--foo--bar"},
				&newProviderDeployWaiterFactory{client: seedClient, waitInterval: ptr.To(20 * time.Millisecond)},
				true,
			)
		}

		hibernateShoot = func(hibernate bool) {
			GinkgoHelper()
			cluster.Spec.Shoot.Object.(*gardencorev1beta1.Shoot).Spec.Hibernation = &gardencorev1beta1.Hibernation{
				Enabled: ptr.To(hibernate),
			}
			Expect(seedClient.Update(ctx, cluster)).To(Succeed(), "failed to update cluster resource")
		}

		runReconcile = func(expectedTargetClass string, expectedProviders ...*dnsv1alpha1.DNSProvider) {
			GinkgoHelper()
			reconcileCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
			dnsController := newDNSControllerMock(reconcileCtx, seedClient, expectedTargetClass, expectedProviders...)
			go dnsController.run()
			Expect(actuator.Reconcile(reconcileCtx, log, ex)).To(Succeed(), "failed to reconcile extension")
			cancel()
			Expect(dnsController.providersReady.Len()).To(Equal(len(expectedProviders)), "expected DNSProviders to become ready")
		}

		checkShootValues = func(useNextGenerationController bool) func(*mockManagedResource) {
			return func(mmr *mockManagedResource) {
				Expect(mmr).ToNot(BeNil())
				Expect(mmr.chartName).To(Equal("shoot-dns-service-shoot"))
				Expect(mmr.class).To(Equal(""))
				Expect(mmr.injectedLabels).To(Equal(map[string]string{"shoot.gardener.cloud/no-cleanup": "true"}))
				checkValues(mmr.values, fmt.Sprintf(`
dnsProviderReplication:
  enabled: true
nextGeneration:
  enabled: %t
serviceName: shoot-dns-service
shootAccessServiceAccountName: extension-shoot-dns-service`, useNextGenerationController))
			}
		}

		checkSeedValues = func(replicas int, useNextGenerationController, restrictToControlPlaneControllers bool) func(*mockManagedResource) {
			return func(mmr *mockManagedResource) {
				Expect(mmr).ToNot(BeNil())
				Expect(mmr.chartName).To(Equal("shoot-dns-service-seed"))
				Expect(mmr.class).To(Equal("seed"))
				Expect(mmr.injectedLabels).To(BeNil())
				checkValues(mmr.values, fmt.Sprintf(`
creatorLabelValue: shoot--foo--bar-78897def-5208-4feb-b0f0-015950ead-32l64aynh256m
dnsClass: source-class
dnsProviderReplication:
  enabled: true
genericTokenKubeconfigSecretName: generic-token-kubeconfig
images:
  dns-controller-manager: '...'
  dns-controller-manager-next-generation: '...'
nextGeneration:
  dnsClass: gardendns-next-gen
  enabled: %t
  restrictToControlPlaneControllers: %t
replicas: %d
seedId: test-seed
serviceName: shoot-dns-service
shootId: shoot--foo--bar-78897def-5208-4feb-b0f0-015950eadbb9-test-landscape
targetClusterSecret: shoot-access-extension-shoot-dns-service`, useNextGenerationController, restrictToControlPlaneControllers, replicas))
			}
		}

		createReplicatedProvider = func(namespace, name string, useNextGenerationController bool, secretName, providerType string) {
			GinkgoHelper()
			provider := &dnsv1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						"gardener.cloud/shoot-id": "shoot--foo--bar-78897def-5208-4feb-b0f0-015950ead-32l64aynh256m",
					},
				},
				Spec: dnsv1alpha1.DNSProviderSpec{
					Type: providerType,
					SecretRef: &corev1.SecretReference{
						Name:      secretName,
						Namespace: namespace,
					},
					Domains: &dnsv1alpha1.DNSSelection{},
					Zones:   &dnsv1alpha1.DNSSelection{},
				},
			}
			if useNextGenerationController {
				provider.Annotations = map[string]string{"dns.gardener.cloud/class": "gardendns-next-gen"}
			}
			Expect(seedClient.Create(ctx, provider)).To(Succeed(), "failed to create replicated DNSProvider")
		}

		createShootProvider = func(namespace, name string, secretName, providerType string) {
			GinkgoHelper()
			provider := &dnsv1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Namespace:   namespace,
					Annotations: map[string]string{"dns.gardener.cloud/class": "garden"},
					Finalizers:  []string{"dns.gardener.cloud/dummy-finalizer"},
				},
				Spec: dnsv1alpha1.DNSProviderSpec{
					Type: providerType,
					SecretRef: &corev1.SecretReference{
						Name:      secretName,
						Namespace: namespace,
					},
					Domains: &dnsv1alpha1.DNSSelection{},
					Zones:   &dnsv1alpha1.DNSSelection{},
				},
			}
			Expect(shootClient.Create(ctx, provider)).To(Succeed(), "failed to create source DNSProvider")
		}

		createEntry = func(namespace, name string, useNextGenerationController bool, domain string, finalizers ...string) {
			GinkgoHelper()
			entry := &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
					Labels: map[string]string{
						"gardener.cloud/shoot-id": "shoot--foo--bar-78897def-5208-4feb-b0f0-015950ead-32l64aynh256m",
					},
					Finalizers: finalizers,
				},
				Spec: dnsv1alpha1.DNSEntrySpec{
					DNSName: domain,
					Targets: []string{"1.2.3.4"},
				},
			}
			if useNextGenerationController {
				entry.Annotations = map[string]string{"dns.gardener.cloud/class": "gardendns-next-gen"}
			}
			Expect(seedClient.Create(ctx, entry)).To(Succeed(), "failed to create DNSEntry")
		}

		createShootEntry = func(namespace, name string, domain string) {
			GinkgoHelper()
			entry := &dnsv1alpha1.DNSEntry{
				ObjectMeta: metav1.ObjectMeta{
					Name:        name,
					Namespace:   namespace,
					Annotations: map[string]string{"dns.gardener.cloud/class": "garden"},
					Finalizers:  []string{"dns.gardener.cloud/dummy-finalizer"},
				},
				Spec: dnsv1alpha1.DNSEntrySpec{
					DNSName: domain,
					Targets: []string{"1.2.3.4"},
				},
			}
			Expect(shootClient.Create(ctx, entry)).To(Succeed(), "failed to create source DNSEntry")
		}

		checkEntryDeleted = func(namespace, name string) {
			GinkgoHelper()
			err := seedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &dnsv1alpha1.DNSEntry{})
			Expect(errors.IsNotFound(err)).To(BeTrue(), "expected DNSEntry to be deleted")
		}

		checkEntryExisting = func(namespace, name string) {
			GinkgoHelper()
			Expect(seedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &dnsv1alpha1.DNSEntry{})).To(Succeed(), "expected DNSEntry to be existing")
		}

		checkProviderDeleted = func(namespace, name string) {
			GinkgoHelper()
			err := seedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &dnsv1alpha1.DNSProvider{})
			Expect(errors.IsNotFound(err)).To(BeTrue(), "expected DNSProvider to be deleted")
		}

		checkProviderExisting = func(namespace, name string) {
			GinkgoHelper()
			Expect(seedClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, &dnsv1alpha1.DNSProvider{})).To(Succeed(), "expected DNSProvider to be existing")
		}

		checkShootEntryDeleted = func(namespace, name string) {
			GinkgoHelper()
			entry := &dnsv1alpha1.DNSEntry{}
			// implicit deletion by deleting CRD does not work with fake client, so we check for finalizers removed
			Expect(shootClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, entry)).To(Succeed(), "expected shoot DNSEntry to be existing")
			Expect(entry.Finalizers).To(BeNil(), "expected shoot DNSEntry to have no finalizers")
		}

		checkShootEntryExisting = func(namespace, name string) {
			GinkgoHelper()
			entry := &dnsv1alpha1.DNSEntry{}
			Expect(shootClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, entry)).To(Succeed(), "expected shoot DNSEntry to be existing")
			Expect(entry.Finalizers).To(ContainElement("dns.gardener.cloud/dummy-finalizer"), "expected shoot DNSEntry to have dummy finalizer")
		}

		checkShootProviderDeleted = func(namespace, name string) {
			GinkgoHelper()
			provider := &dnsv1alpha1.DNSProvider{}
			// implicit deletion by deleting CRD does not work with fake client, so we check for finalizers removed
			Expect(shootClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, provider)).To(Succeed(), "expected shoot DNSProvider to be existing")
			Expect(provider.Finalizers).To(BeNil(), "expected shoot DNSProvider to have no finalizers")
		}

		checkShootProviderExisting = func(namespace, name string) {
			GinkgoHelper()
			provider := &dnsv1alpha1.DNSProvider{}
			Expect(shootClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, provider)).To(Succeed(), "expected shoot DNSProvider to be existing")
			Expect(provider.Finalizers).To(ContainElement("dns.gardener.cloud/dummy-finalizer"), "expected shoot DNSProvider to have dummy finalizer")
		}

		specialConfigValues = func(useNextGenerationController bool) (string, int) {
			targetClass := ""
			expectedReplicasOnCleaningUp := 0
			if useNextGenerationController {
				targetClass = "gardendns-next-gen"
				expectedReplicasOnCleaningUp = 1
			}
			return targetClass, expectedReplicasOnCleaningUp
		}

		checkStandardReconciliation = func(useNextGenerationController bool) {
			targetClass, _ := specialConfigValues(useNextGenerationController)
			prepareControlPlane(useNextGenerationController)

			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-shoot",
				checkShootValues(useNextGenerationController))
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(1, useNextGenerationController, false))
			runReconcile(targetClass, providerExternal, providerAdditional)

			managedResourcesAccess.ConsumedAllExpectations()
		}

		checkHibernation = func(useNextGenerationController bool) {
			targetClass, expectedReplicasOnCleaningUp := specialConfigValues(useNextGenerationController)

			createReplicatedProvider("shoot--foo--bar", "source-provider", useNextGenerationController, "source-provider-secret", "aws-route53")
			createEntry("shoot--foo--bar", "some-entry", useNextGenerationController, "foo.bar.external.example.com")

			By("Hibernation")
			hibernateShoot(true)

			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-shoot",
				checkShootValues(useNextGenerationController))
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(0, useNextGenerationController, false))
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(expectedReplicasOnCleaningUp, useNextGenerationController, true),
				func(_ *mockManagedResource) {
					checkEntryExisting("shoot--foo--bar", "some-entry")
					checkProviderExisting("shoot--foo--bar", "source-provider")
				})
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(expectedReplicasOnCleaningUp, useNextGenerationController, true),
				func(_ *mockManagedResource) {
					checkEntryDeleted("shoot--foo--bar", "some-entry")
					checkProviderExisting("shoot--foo--bar", "source-provider")
				})
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(0, useNextGenerationController, false),
				func(_ *mockManagedResource) {
					checkProviderDeleted("shoot--foo--bar", "source-provider")
				})
			runReconcile(targetClass)

			By("Hibernation - second round without DNSEntry, but with replicated DNSProvider")
			createReplicatedProvider("shoot--foo--bar", "source-provider", useNextGenerationController, "source-provider-secret", "aws-route53")
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-shoot",
				checkShootValues(useNextGenerationController))
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(0, useNextGenerationController, false))
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(expectedReplicasOnCleaningUp, useNextGenerationController, true),
				func(_ *mockManagedResource) {
					checkProviderExisting("shoot--foo--bar", "source-provider")
				})
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(0, useNextGenerationController, false),
				func(_ *mockManagedResource) {
					checkProviderDeleted("shoot--foo--bar", "source-provider")
				})
			runReconcile(targetClass)

			By("Hibernation - third round without DNSEntry or replicated DNSProvider")
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-shoot",
				checkShootValues(useNextGenerationController))
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(0, useNextGenerationController, false))
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(0, useNextGenerationController, false))
			runReconcile(targetClass)
		}

		prepareShootCRDsAndCRs = func() {
			GinkgoHelper()
			Expect(dnsapp.DeployCRDsWithClient(ctx, log, shootClient, &dnsapisconfig.DNSManagerConfiguration{
				DeployCRDs: ptr.To(true),
				Controllers: dnsapisconfig.ControllerConfiguration{
					Source: dnsapisconfig.SourceControllerConfig{
						DNSProviderReplication: ptr.To(true),
					},
				},
			})).To(Succeed(), "failed to deploy DNS CRDs to shoot cluster")
			createShootProvider("default", "shoot-provider", "shoot-provider-secret", "aws-route53")
			createShootEntry("default", "shoot-entry", "foo.bar.external.example.com")
		}

		checkShootCRDsAndCRsExisting = func() {
			GinkgoHelper()
			checkShootProviderExisting("default", "shoot-provider")
			checkShootEntryExisting("default", "shoot-entry")
			list := &apiextensionsv1.CustomResourceDefinitionList{}
			Expect(shootClient.List(ctx, list)).To(Succeed(), "failed to list CRDs in shoot cluster")
			Expect(list.Items).To(HaveLen(3), "expected DNS CRDs in shoot cluster")
		}

		checkShootCRDsAndCRsDeleted = func() {
			GinkgoHelper()
			checkShootProviderDeleted("default", "shoot-provider")
			checkShootEntryDeleted("default", "shoot-entry")

			list := &apiextensionsv1.CustomResourceDefinitionList{}
			Expect(shootClient.List(ctx, list)).To(Succeed(), "failed to list CRDs in shoot cluster")
			Expect(list.Items).To(BeEmpty(), "expected no CRDs in shoot cluster")
		}

		checkDeletion = func(migrate, useNextGenerationController bool,
			checkFuncs ...func(*mockManagedResource)) {
			GinkgoHelper()
			_, expectedReplicasOnCleaningUp := specialConfigValues(useNextGenerationController)

			createReplicatedProvider("shoot--foo--bar", "source-provider", useNextGenerationController, "source-provider-secret", "aws-route53")
			createEntry("shoot--foo--bar", "some-entry", useNextGenerationController, "foo.bar.external.example.com")
			prepareShootCRDsAndCRs()

			By("Delete")
			if migrate {
				managedResourcesAccess.ExpectCall(managedResourceCallSetKeepObjects, "shoot--foo--bar", "extension-shoot-dns-service-shoot",
					func(mmr *mockManagedResource) {
						Expect(mmr.keepObjects).To(BeTrue(), "expected keepObjects to be true")
					})
			} else {
				managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
					checkSeedValues(expectedReplicasOnCleaningUp, useNextGenerationController, true),
					func(mmr *mockManagedResource) {
						checkEntryExisting("shoot--foo--bar", "some-entry")
						checkProviderExisting("shoot--foo--bar", "source-provider")
						checkShootCRDsAndCRsExisting()
						for _, checkFunc := range checkFuncs {
							checkFunc(mmr)
						}
					})
			}
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(expectedReplicasOnCleaningUp, useNextGenerationController, true),
				func(_ *mockManagedResource) {
					if migrate {
						checkEntryExisting("shoot--foo--bar", "some-entry")
						checkShootCRDsAndCRsExisting()
					} else {
						checkEntryDeleted("shoot--foo--bar", "some-entry")
						checkShootCRDsAndCRsDeleted()
					}
					checkProviderExisting("shoot--foo--bar", "source-provider")
				})
			managedResourcesAccess.ExpectDelete("shoot--foo--bar", "extension-shoot-dns-service-seed",
				func(_ *mockManagedResource) {
					if migrate {
						checkEntryExisting("shoot--foo--bar", "some-entry")
					} else {
						checkEntryDeleted("shoot--foo--bar", "some-entry")
					}
					checkProviderDeleted("shoot--foo--bar", "source-provider")
				})
			managedResourcesAccess.ExpectWaitUntilDeleted("shoot--foo--bar", "extension-shoot-dns-service-seed")
			managedResourcesAccess.ExpectDelete("shoot--foo--bar", "extension-shoot-dns-service-shoot")
			managedResourcesAccess.ExpectWaitUntilDeleted("shoot--foo--bar", "extension-shoot-dns-service-shoot")
			reconcileCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
			if migrate {
				Expect(actuator.Migrate(reconcileCtx, log, ex)).To(Succeed())
			} else {
				Expect(actuator.Delete(reconcileCtx, log, ex)).To(Succeed())
			}
			cancel()
		}

		checkForceDeletion = func(useNextGenerationController bool) {
			_, expectedReplicasOnCleaningUp := specialConfigValues(useNextGenerationController)

			createReplicatedProvider("shoot--foo--bar", "source-provider", useNextGenerationController, "source-provider-secret", "aws-route53")
			createEntry("shoot--foo--bar", "some-entry", useNextGenerationController, "foo.bar.external.example.com")
			createEntry("shoot--foo--bar", "stuck-entry", useNextGenerationController, "stuck.bar.external.example.com", "dns.gardener.cloud/dummy-finalizer")

			By("Force Delete")
			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(expectedReplicasOnCleaningUp, useNextGenerationController, true),
				func(mmr *mockManagedResource) {
					checkEntryExisting("shoot--foo--bar", "some-entry")
					checkEntryExisting("shoot--foo--bar", "stuck-entry")
					checkProviderExisting("shoot--foo--bar", "source-provider")
				})
			managedResourcesAccess.ExpectDelete("shoot--foo--bar", "extension-shoot-dns-service-seed",
				func(_ *mockManagedResource) {
					checkEntryDeleted("shoot--foo--bar", "some-entry")
					checkEntryExisting("shoot--foo--bar", "stuck-entry")
					checkProviderExisting("shoot--foo--bar", "source-provider")
				})
			managedResourcesAccess.ExpectWaitUntilDeleted("shoot--foo--bar", "extension-shoot-dns-service-seed")

			managedResourcesAccess.ExpectCreateOrUpdate("shoot--foo--bar", "extension-shoot-dns-service-seed",
				checkSeedValues(expectedReplicasOnCleaningUp, useNextGenerationController, true),
				func(mmr *mockManagedResource) {
					checkEntryDeleted("shoot--foo--bar", "some-entry")
					checkEntryDeleted("shoot--foo--bar", "stuck-entry")
					checkProviderExisting("shoot--foo--bar", "source-provider")
				})

			managedResourcesAccess.ExpectDelete("shoot--foo--bar", "extension-shoot-dns-service-seed",
				func(mmr *mockManagedResource) {
					checkEntryDeleted("shoot--foo--bar", "some-entry")
					checkEntryDeleted("shoot--foo--bar", "stuck-entry")
					checkProviderDeleted("shoot--foo--bar", "source-provider")
				})
			managedResourcesAccess.ExpectWaitUntilDeleted("shoot--foo--bar", "extension-shoot-dns-service-seed")
			reconcileCtx, cancel := context.WithTimeout(ctx, 100*time.Second)
			Expect(actuator.ForceDelete(reconcileCtx, log, ex)).To(Succeed())
			cancel()
		}

		checkDNSEntriesIgnored = func(*mockManagedResource) {
			GinkgoHelper()
			entry := &dnsv1alpha1.DNSEntry{}
			Expect(seedClient.Get(ctx, client.ObjectKey{Namespace: "shoot--foo--bar", Name: "some-entry"}, entry)).To(Succeed(), "expected DNSEntry to be existing")
			Expect(entry.Annotations["dns.gardener.cloud/target-hard-ignore"]).To(Equal("true"), "expected DNSEntry to have hard-ignore annotation")
		}
	)

	BeforeEach(func() {
		ctx = context.Background()
		logf.SetLogger(logger.MustNewZapLogger(logger.DebugLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))
		log = logf.Log.WithName("test")
		dnsServiceConfig = config.DNSServiceConfig{
			SeedID:                "test-seed",
			DNSClass:              "source-class",
			ManageDNSProviders:    true,
			ReplicateDNSProviders: true,
		}

		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).NotTo(HaveOccurred())
		Expect(dnsv1alpha1.AddToScheme(scheme)).NotTo(HaveOccurred())
		Expect(apisservice.AddToScheme(scheme)).NotTo(HaveOccurred())
		Expect(servicev1alpha1.AddToScheme(scheme)).NotTo(HaveOccurred())
		Expect(extensionscontroller.AddToScheme(scheme)).NotTo(HaveOccurred())
		Expect(apiextensionsv1.AddToScheme(scheme)).NotTo(HaveOccurred())

		decoder = serializer.NewCodecFactory(scheme).UniversalDecoder()

		seedClient = fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&extensionsv1alpha1.Extension{}, &dnsv1alpha1.DNSProvider{}).Build()
		shootClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		managedResourcesAccess = newTestManagedResourcesAccess()

		cluster = &extensionsv1alpha1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "shoot--foo--bar",
			},
			Spec: extensionsv1alpha1.ClusterSpec{
				Shoot: runtime.RawExtension{
					Object: &gardencorev1beta1.Shoot{
						TypeMeta: metav1.TypeMeta{
							APIVersion: "core.gardener.cloud/v1beta1",
							Kind:       "Shoot",
						},
						ObjectMeta: metav1.ObjectMeta{
							Name:      "bar",
							Namespace: "garden-foo",
						},
						Spec: gardencorev1beta1.ShootSpec{
							DNS: &gardencorev1beta1.DNS{
								Domain: ptr.To("foo.bar.external.example.com"),
							},
							Extensions: []gardencorev1beta1.Extension{
								{
									Type: "shoot-dns-service",
									ProviderConfig: &runtime.RawExtension{
										Object: &servicev1alpha1.DNSConfig{
											TypeMeta: metav1.TypeMeta{
												APIVersion: "service.dns.extensions.gardener.cloud/v1alpha1",
												Kind:       "DNSConfig",
											},
											Providers: []servicev1alpha1.DNSProvider{
												{
													SecretName: ptr.To(providerSecretName),
													Type:       ptr.To("aws-route53"),
												},
											},
											DNSProviderReplication: &servicev1alpha1.DNSProviderReplication{
												Enabled: true,
											},
											SyncProvidersFromShootSpecDNS: ptr.To(false),
										},
									},
								},
							},
							Kubernetes: gardencorev1beta1.Kubernetes{
								Version: "1.33.1",
							},
							Resources: []gardencorev1beta1.NamedResourceReference{
								{
									Name: providerSecretName,
									ResourceRef: autoscalingv1.CrossVersionObjectReference{
										APIVersion: "v1",
										Kind:       "Secret",
										Name:       "original-" + providerSecretName,
									},
								},
							},
						},
						Status: gardencorev1beta1.ShootStatus{
							ClusterIdentity: ptr.To("shoot--foo--bar-78897def-5208-4feb-b0f0-015950eadbb9-test-landscape"),
						},
					},
				},
			},
		}

		providerExternal = &dnsv1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external",
				Namespace: cluster.Name,
			},
			Spec: dnsv1alpha1.DNSProviderSpec{
				Type: "google-clouddns",
				SecretRef: &corev1.SecretReference{
					Name:      "external-dns-secret",
					Namespace: cluster.Name,
				},
				Domains: &dnsv1alpha1.DNSSelection{
					Include: []string{"foo.bar.external.example.com"},
					Exclude: []string{"api.foo.bar.external.example.com"},
				},
				Zones: &dnsv1alpha1.DNSSelection{
					Include: []string{"zone-12345"},
				},
			},
		}

		providerAdditional = &dnsv1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "aws-route53-provider-secret",
				Namespace: cluster.Name,
			},
			Spec: dnsv1alpha1.DNSProviderSpec{
				Type: "aws-route53",
				SecretRef: &corev1.SecretReference{
					Name:      "ref-original-" + providerSecretName,
					Namespace: cluster.Name,
				},
				Domains: &dnsv1alpha1.DNSSelection{},
				Zones:   &dnsv1alpha1.DNSSelection{},
			},
		}
	})

	AfterEach(func() {
		managedResourcesAccess.ConsumedAllExpectations()
	})

	Describe("#Reconcile/#Restore", func() {
		It("should reconcile the extension with useNextGenerationController != true", func() {
			checkStandardReconciliation(false)
			checkHibernation(false)
		})

		It("should reconcile the extension with useNextGenerationController == true", func() {
			checkStandardReconciliation(true)
			checkHibernation(true)
		})
	})

	Describe("#Delete", func() {
		It("should delete the extension with useNextGenerationController != true", func() {
			checkStandardReconciliation(false)
			checkDeletion(false, false)
		})

		It("should delete the extension with useNextGenerationController == true", func() {
			checkStandardReconciliation(true)
			checkDeletion(false, true)
		})
	})

	Describe("#ForceDelete", func() {
		It("should force delete the extension with useNextGenerationController != true", func() {
			checkStandardReconciliation(false)
			checkForceDeletion(false)
		})

		It("should delete the extension with useNextGenerationController == true", func() {
			checkStandardReconciliation(true)
			checkForceDeletion(true)
		})
	})

	Describe("#Migrate", func() {
		It("should migrate the extension with useNextGenerationController != true", func() {
			checkStandardReconciliation(false)
			checkDeletion(true, false, checkDNSEntriesIgnored)
		})

		It("should migrate the extension with useNextGenerationController == true", func() {
			checkStandardReconciliation(true)
			checkDeletion(true, true, checkDNSEntriesIgnored)
		})
	})
})

func checkValues(values map[string]any, expectedYAML string) {
	GinkgoHelper()
	if values["images"] != nil {
		imageMap := values["images"].(map[string]any)
		for _, key := range []string{"dns-controller-manager", "dns-controller-manager-next-generation"} {
			imageMap[key] = "..."
		}
	}
	data, err := yaml.Marshal(values)
	Expect(err).ToNot(HaveOccurred(), "failed to marshal chart values to YAML")
	Expect(strings.TrimSpace(string(data))).To(Equal(strings.TrimSpace(expectedYAML)), "chart values do not match expected YAML")
}
