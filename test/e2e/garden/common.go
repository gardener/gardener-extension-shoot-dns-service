// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package garden

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	operatorv1alpha1 "github.com/gardener/gardener/pkg/apis/operator/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/logger"
	. "github.com/gardener/gardener/pkg/utils/test"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	kubernetesscheme "k8s.io/client-go/kubernetes/scheme"
	componentbaseconfigv1alpha1 "k8s.io/component-base/config/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	parentCtx     context.Context
	runtimeClient client.Client
)

var _ = BeforeSuite(func() {
	Expect(os.Getenv("KUBECONFIG")).NotTo(BeEmpty(), "KUBECONFIG must be set")
	Expect(os.Getenv("REPO_ROOT")).NotTo(BeEmpty(), "REPO_ROOT must be set")

	logf.SetLogger(logger.MustNewZapLogger(logger.InfoLevel, logger.FormatJSON, zap.WriteTo(GinkgoWriter)))

	restConfig, err := kubernetes.RESTConfigFromClientConnectionConfiguration(&componentbaseconfigv1alpha1.ClientConnectionConfiguration{Kubeconfig: os.Getenv("KUBECONFIG")}, nil, kubernetes.AuthTokenFile, kubernetes.AuthClientCertificate)
	Expect(err).NotTo(HaveOccurred())

	scheme := runtime.NewScheme()
	Expect(kubernetesscheme.AddToScheme(scheme)).To(Succeed())
	Expect(operatorv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(extensionsv1alpha1.AddToScheme(scheme)).To(Succeed())
	Expect(dnsv1alpha1.AddToScheme(scheme)).To(Succeed())
	runtimeClient, err = client.New(restConfig, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
})

var _ = BeforeEach(func() {
	parentCtx = context.Background()
})

func waitForShootToBeReconciled(ctx context.Context, gardenClient client.Client, shoot *gardencorev1beta1.Shoot) {
	CEventually(ctx, func(g Gomega) gardencorev1beta1.LastOperationState {
		g.Expect(gardenClient.Get(ctx, client.ObjectKeyFromObject(shoot), shoot)).To(Succeed())
		if shoot.Status.LastOperation == nil || shoot.Status.ObservedGeneration != shoot.Generation {
			return ""
		}
		return shoot.Status.LastOperation.State
	}).WithPolling(2 * time.Second).Should(Equal(gardencorev1beta1.LastOperationStateSucceeded))
}

func waitForShootReconciliationToBeProcessing(ctx context.Context, gardenClient client.Client, shoot *gardencorev1beta1.Shoot, minProgress int32) {
	CEventually(ctx, func(g Gomega) {
		g.Expect(gardenClient.Get(ctx, client.ObjectKeyFromObject(shoot), shoot)).To(Succeed())
		g.Expect(shoot.Status.LastOperation == nil || shoot.Status.ObservedGeneration != shoot.Generation).To(BeFalse(), "shoot reconciliation has not started yet")
		g.Expect(shoot.Status.LastOperation.Type).To(Or(Equal(gardencorev1beta1.LastOperationTypeCreate), Equal(gardencorev1beta1.LastOperationTypeReconcile)))
		g.Expect(shoot.Status.LastOperation.State).To(Or(Equal(gardencorev1beta1.LastOperationStateProcessing), Equal(gardencorev1beta1.LastOperationStateSucceeded)))
		g.Expect(shoot.Status.LastOperation.Progress).To(BeNumerically(">=", minProgress))
	}).WithPolling(20 * time.Millisecond).Should(Succeed())
}

func patchExternalProvider(ctx context.Context, providerKey client.ObjectKey) {
	CEventually(ctx, func(g Gomega) {
		provider := &dnsv1alpha1.DNSProvider{}
		g.Expect(runtimeClient.Get(ctx, providerKey, provider)).To(Succeed())
		patch := client.MergeFrom(provider.DeepCopy())
		provider.Status.State = "Ready"
		provider.Status.ObservedGeneration = provider.Generation
		g.Expect(runtimeClient.SubResource("status").Patch(ctx, provider, patch)).To(Succeed())
	}).Should(Succeed())
}

func waitForOperatorExtensionToBeReconciled(ctx context.Context, extension *operatorv1alpha1.Extension) {
	CEventually(ctx, func(g Gomega) []gardencorev1beta1.Condition {
		g.Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(extension), extension)).To(Succeed())
		if extension.Status.ObservedGeneration != extension.Generation {
			return nil
		}
		return extension.Status.Conditions
	}).WithPolling(1 * time.Second).Should(ContainElements(MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(operatorv1alpha1.ExtensionInstalled),
		"Status": Equal(gardencorev1beta1.ConditionTrue),
	}), MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(gardencorev1beta1.ConditionType(operatorv1alpha1.ControllerInstallationsHealthy)),
		"Status": Equal(gardencorev1beta1.ConditionTrue),
	}), MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(operatorv1alpha1.ExtensionRequiredRuntime),
		"Status": Equal(gardencorev1beta1.ConditionFalse),
	}), MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(operatorv1alpha1.ExtensionAdmissionHealthy),
		"Status": Equal(gardencorev1beta1.ConditionTrue),
	}), MatchFields(IgnoreExtras, Fields{
		"Type":   Equal(operatorv1alpha1.ExtensionRequiredVirtual),
		"Status": Equal(gardencorev1beta1.ConditionTrue),
	})))
}

func waitForProviderReady(ctx context.Context, shootClient client.Client, provider *dnsv1alpha1.DNSProvider, expectedDomain string) {
	CEventually(ctx, func(g Gomega) {
		g.Expect(shootClient.Get(ctx, client.ObjectKeyFromObject(provider), provider)).To(Succeed())
		g.Expect(provider.Status.State).To(Equal("Ready"))
		g.Expect(provider.Status.Domains.Included).To(ContainElement(expectedDomain))
		g.Expect(provider.Status.Zones.Included).To(ContainElement(expectedDomain + "."))
		g.Expect(provider.Finalizers).To(ContainElement("garden.dns.gardener.cloud/dnsprovider-replication"))
	}).WithPolling(1 * time.Second).Should(Succeed())
}

func deleteShoot(ctx context.Context, gardenClient client.Client, shoot *gardencorev1beta1.Shoot) {
	patch := client.MergeFrom(shoot.DeepCopy())
	if shoot.Annotations == nil {
		shoot.Annotations = make(map[string]string)
	}
	shoot.Annotations["confirmation.gardener.cloud/deletion"] = "true"
	Expect(gardenClient.Patch(ctx, shoot, patch)).To(Succeed())
	Expect(gardenClient.Delete(ctx, shoot)).To(Succeed())
}

func waitForShootToBeDeleted(ctx context.Context, gardenClient client.Client, shoot *gardencorev1beta1.Shoot) {
	CEventually(ctx, func(g Gomega) bool {
		err := gardenClient.Get(ctx, client.ObjectKeyFromObject(shoot), shoot)
		if err != nil {
			return apierrors.IsNotFound(err)
		}
		return false
	}).WithPolling(1 * time.Second).WithTimeout(10 * time.Minute).Should(BeTrue())
}

func waitForShootDNSEntryReady(ctx context.Context, shootClient client.Client, dnsEntry *dnsv1alpha1.DNSEntry) {
	CEventually(ctx, func(g Gomega) string {
		g.Expect(shootClient.Get(ctx, client.ObjectKeyFromObject(dnsEntry), dnsEntry)).To(Succeed())
		g.Expect(dnsEntry.Status.ObservedGeneration).To(Equal(dnsEntry.Generation))
		g.Expect(dnsEntry.Finalizers).To(ContainElement("garden.dns.gardener.cloud/dnsentry-source"))
		return dnsEntry.Status.State
	}).WithPolling(1 * time.Second).Should(Equal("Ready"))
}

// ExecMake executes one or multiple make targets.
func execMake(ctx context.Context, targets ...string) error {
	cmd := exec.CommandContext(ctx, "make", targets...)
	cmd.Dir = os.Getenv("REPO_ROOT")
	for _, key := range []string{"PATH", "GOPATH", "HOME", "KUBECONFIG"} {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, os.Getenv(key)))
	}
	cmdString := fmt.Sprintf("running make %s", strings.Join(targets, " "))
	logf.Log.Info(cmdString)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %s\n%s", cmdString, err, string(output))
	}
	return nil
}
