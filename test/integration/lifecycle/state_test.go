// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle_test

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	resourcesv1alpha1 "github.com/gardener/gardener/pkg/apis/resources/v1alpha1"
	"github.com/gardener/gardener/pkg/logger"
	gardenerutils "github.com/gardener/gardener/pkg/utils"
	. "github.com/gardener/gardener/pkg/utils/test"
	"github.com/gardener/gardener/test/framework"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/json"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/common"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/config"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/controller/lifecycle"
)

var (
	entryCount = flag.Int("entries", 10, "Number of DNS entries to create")
	logLevel   = flag.String("logLevel", "", "Log level (debug, info, error)")
)

const (
	defaultTimeout = 30 * time.Second
)

func validateFlags() {
	if len(*logLevel) == 0 {
		logLevel = ptr.To(logger.DebugLevel)
	} else {
		if !slices.Contains(logger.AllLogLevels, *logLevel) {
			panic("invalid log level: " + *logLevel)
		}
	}
	if *entryCount < 1 {
		panic("invalid entry count: " + fmt.Sprint(*entryCount))
	}
}

var (
	ctx = context.Background()

	log       logr.Logger
	testEnv   *envtest.Environment
	mgrCancel context.CancelFunc
	c         client.Client

	testName string

	namespace *corev1.Namespace
	entries   []*dnsv1alpha1.DNSEntry
	shoot     *gardencorev1beta1.Shoot
	cluster   *extensionsv1alpha1.Cluster
)

var _ = BeforeSuite(func() {
	flag.Parse()
	validateFlags()

	repoRoot := filepath.Join("..", "..", "..")

	// enable manager logs
	logf.SetLogger(logger.MustNewZapLogger(*logLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))

	log = logf.Log.WithName("lifecycle-test")

	config.DNSService.SeedID = "test-seed"
	config.DNSService.ManageDNSProviders = true
	DeferCleanup(func() {
		By("stopping manager")
		mgrCancel()

		By("running cleanup actions")
		framework.RunCleanupActions()

		By("tearing down shoot environment")
		teardownShootEnvironment(ctx, c, namespace, entries, cluster)

		By("stopping test environment")
		Expect(testEnv.Stop()).To(Succeed())
		config.DNSService.SeedID = ""
		config.DNSService.ManageDNSProviders = false
	})

	By("generating randomized test resource identifiers")
	testName = fmt.Sprintf("shoot--foo--%s", randomString())
	namespace = &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: testName,
		},
	}
	shoot = &gardencorev1beta1.Shoot{
		Spec: gardencorev1beta1.ShootSpec{
			DNS: &gardencorev1beta1.DNS{
				Domain: ptr.To(testName + "example.com"),
			},
			Kubernetes: gardencorev1beta1.Kubernetes{
				Version: "1.31.0",
			},
		},
		Status: gardencorev1beta1.ShootStatus{
			ClusterIdentity: ptr.To(testName + "-12345678"),
		},
	}
	cluster = &extensionsv1alpha1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: testName,
		},
		Spec: extensionsv1alpha1.ClusterSpec{
			CloudProfile: runtime.RawExtension{Raw: []byte("{}")},
			Seed:         runtime.RawExtension{Raw: []byte("{}")},
			Shoot:        runtime.RawExtension{Raw: shootToBytes(shoot)},
		},
	}
	entries = createDNSEntries(*shoot.Status.ClusterIdentity, testName, *entryCount)

	By("starting test environment")
	testEnv = &envtest.Environment{
		CRDInstallOptions: envtest.CRDInstallOptions{
			Paths: []string{
				filepath.Join(repoRoot, "example", "11-resource-manager-crds.yaml"),
				filepath.Join(repoRoot, "example", "20-crds.yaml"),
			},
		},
	}

	restConfig, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(restConfig).ToNot(BeNil())

	By("setting up manager")
	mgr, err := manager.New(restConfig, manager.Options{
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(extensionsv1alpha1.AddToScheme(mgr.GetScheme())).To(Succeed())
	Expect(dnsv1alpha1.AddToScheme(mgr.GetScheme())).To(Succeed())
	Expect(resourcesv1alpha1.AddToScheme(mgr.GetScheme())).To(Succeed())

	Expect(lifecycle.AddToManagerWithOptions(ctx, mgr, lifecycle.AddOptions{})).To(Succeed())

	var mgrContext context.Context
	mgrContext, mgrCancel = context.WithCancel(ctx)

	By("starting manager")
	go func() {
		defer GinkgoRecover()
		err := mgr.Start(mgrContext)
		Expect(err).NotTo(HaveOccurred())
	}()

	// test client should be uncached and independent from the tested manager
	c, err = client.New(restConfig, client.Options{Scheme: mgr.GetScheme()})
	Expect(err).NotTo(HaveOccurred())
	Expect(c).NotTo(BeNil())

	By("setting up shoot environment")
	setupShootEnvironment(ctx, c, namespace, entries, cluster)
})

var _ = Describe("Lifecycle state tests", func() {
	It("it should update the extension status state", func() {
		By("creating extension")
		ext := &extensionsv1alpha1.Extension{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: testName,
			},
			Spec: extensionsv1alpha1.ExtensionSpec{
				DefaultSpec: extensionsv1alpha1.DefaultSpec{
					Type: "shoot-dns-service",
				},
			},
		}
		Expect(c.Create(ctx, ext)).To(Succeed())

		By("wait for 'external' DNSProvider and patch it to Ready")
		CEventually(ctx, func(g Gomega) error {
			provider := &dnsv1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: testName,
					Name:      "external",
				},
			}
			g.Expect(c.Get(ctx, client.ObjectKeyFromObject(provider), provider)).To(Succeed())

			patch := client.MergeFrom(provider.DeepCopy())
			provider.Status.State = "Ready"
			provider.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}
			provider.Status.ObservedGeneration = provider.Generation
			return c.Status().Patch(ctx, provider, patch)
		}).Should(Succeed())

		By("waiting for extension last operation to succeed")
		CEventually(ctx, func() bool {
			Expect(c.Get(ctx, client.ObjectKeyFromObject(ext), ext)).To(Succeed())
			return ext.Status.LastOperation != nil && ext.Status.LastOperation.State == gardencorev1beta1.LastOperationStateSucceeded
		}).WithPolling(1 * time.Second).WithTimeout(defaultTimeout).Should(BeTrue())

		By("check for managed resources")
		mr := &resourcesv1alpha1.ManagedResource{}
		Expect(c.Get(ctx, client.ObjectKey{Namespace: testName, Name: "extension-shoot-dns-service-seed"}, mr)).To(Succeed())
		Expect(c.Get(ctx, client.ObjectKey{Namespace: testName, Name: "extension-shoot-dns-service-shoot"}, mr)).To(Succeed())

		By("check empty compressed extension state")
		Expect(c.Get(ctx, client.ObjectKeyFromObject(ext), ext)).To(Succeed())
		Expect(ext.Status.State).NotTo(BeNil())
		Expect(common.LooksLikeCompressedEntriesState(ext.Status.State.Raw)).Should(BeTrue())

		state, err := common.GetExtensionState(ext)
		Expect(err).NotTo(HaveOccurred())
		Expect(state).NotTo(BeNil())
		Expect(state.Entries).To(HaveLen(0))

		By("start migration")
		CEventually(ctx, func(g Gomega) error {
			patch := client.MergeFrom(ext.DeepCopy())
			if ext.Annotations == nil {
				ext.Annotations = map[string]string{}
			}
			ext.Annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationMigrate
			return c.Patch(ctx, ext, patch)
		}).Should(Succeed())

		By("waiting for extension last operation to succeed")
		CEventually(ctx, func() bool {
			Expect(c.Get(ctx, client.ObjectKeyFromObject(ext), ext)).To(Succeed())
			return ext.Status.LastOperation != nil &&
				ext.Status.LastOperation.State == gardencorev1beta1.LastOperationStateSucceeded &&
				ext.Status.LastOperation.Type == gardencorev1beta1.LastOperationTypeMigrate
		}).WithPolling(1 * time.Second).WithTimeout(defaultTimeout).Should(BeTrue())

		By("check non-empty compressed extension state")
		Expect(common.LooksLikeCompressedEntriesState(ext.Status.State.Raw)).Should(BeTrue())
		compressedSize := len(ext.Status.State.Raw)
		uncompressed, err := common.DecompressEntriesState(ext.Status.State.Raw)
		Expect(err).NotTo(HaveOccurred())

		state, err = common.GetExtensionState(ext)
		Expect(err).NotTo(HaveOccurred())
		Expect(state).NotTo(BeNil())
		Expect(state.Entries).To(HaveLen(len(entries)))

		By("check uncompressed extension state")
		ext.Status.State.Raw = uncompressed
		state2, err := common.GetExtensionState(ext)
		Expect(err).NotTo(HaveOccurred())
		Expect(state2).NotTo(BeNil())
		Expect(state).To(Equal(state2))

		log.Info("compressed rate", "rate", fmt.Sprintf("%.1f %%", 100.0*float32(compressedSize)/float32(len(uncompressed))))

		By("deleting extension")
		Expect(c.Delete(ctx, ext)).To(Succeed())
		CEventually(ctx, func() bool {
			err := c.Get(ctx, client.ObjectKeyFromObject(ext), ext)
			return err != nil && client.IgnoreNotFound(err) == nil
		}).WithPolling(1 * time.Second).WithTimeout(defaultTimeout).Should(BeTrue())
	})
})

func setupShootEnvironment(ctx context.Context, c client.Client, namespace *corev1.Namespace, entries []*dnsv1alpha1.DNSEntry, cluster *extensionsv1alpha1.Cluster) {
	Expect(c.Create(ctx, namespace)).To(Succeed())
	for _, entry := range entries {
		status := *entry.Status.DeepCopy()
		Expect(c.Create(ctx, entry)).To(Succeed())
		entry.Status = status
		entry.Status.ObservedGeneration = entry.Generation
		Expect(c.SubResource("status").Update(ctx, entry)).To(Succeed())
	}
	Expect(c.Create(ctx, cluster)).To(Succeed())
}

func teardownShootEnvironment(ctx context.Context, c client.Client, namespace *corev1.Namespace, entries []*dnsv1alpha1.DNSEntry, cluster *extensionsv1alpha1.Cluster) {
	Expect(client.IgnoreNotFound(c.Delete(ctx, cluster))).To(Succeed())
	for _, entry := range entries {
		Expect(client.IgnoreNotFound(c.Delete(ctx, entry))).To(Succeed())
	}
	Expect(c.DeleteAllOf(ctx, &corev1.Secret{}, client.InNamespace(namespace.Name), client.MatchingLabels{"resources.gardener.cloud/garbage-collectable-reference": "true"})).To(Succeed())
	Expect(client.IgnoreNotFound(c.Delete(ctx, namespace))).To(Succeed())
}

func createDNSEntries(shootID, namespace string, count int) []*dnsv1alpha1.DNSEntry {
	entries := make([]*dnsv1alpha1.DNSEntry, count)
	for i := range count {
		name := fmt.Sprintf("dnsentry-%d", i)
		entries[i] = &dnsv1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
				Labels: map[string]string{
					common.ShootDNSEntryLabelKey: shootID,
				},
			},
			Spec: dnsv1alpha1.DNSEntrySpec{
				DNSName: name + "some.blabla.example.com",
				TTL:     ptr.To[int64](300),
			},
			Status: dnsv1alpha1.DNSEntryStatus{
				Provider:       ptr.To("shoot--foo--barbar/some-test-provider"),
				LastUpdateTime: ptr.To(metav1.Now()),
				ProviderType:   ptr.To("aws-route53"),
				State:          "Ready",
				Targets:        []string{fmt.Sprintf("f00aa479c3011153f4bdd5f65b89e7ff-f000-%04x.elb.eu-central-1.amazonaws.com", i)},
				TTL:            ptr.To[int64](300),
				Zone:           ptr.To("ZFOOBAR4ROWB4VQ"),
			},
		}
	}
	return entries
}

func randomString() string {
	rs, err := gardenerutils.GenerateRandomStringFromCharset(5, "0123456789abcdefghijklmnopqrstuvwxyz")
	Expect(err).NotTo(HaveOccurred())
	return rs
}

func shootToBytes(shoot *gardencorev1beta1.Shoot) []byte {
	data, err := json.Marshal(shoot)
	Expect(err).NotTo(HaveOccurred())
	return data
}
