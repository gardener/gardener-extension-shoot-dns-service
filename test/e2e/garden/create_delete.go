// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package garden

import (
	"context"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	operatorv1alpha1 "github.com/gardener/gardener/pkg/apis/operator/v1alpha1"
	"github.com/gardener/gardener/pkg/client/kubernetes"
	operatorclient "github.com/gardener/gardener/pkg/operator/client"
	"github.com/gardener/gardener/test/utils/access"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var _ = Describe("Shoot-DNS-Service Tests", func() {
	var (
		operatorExtension = &operatorv1alpha1.Extension{ObjectMeta: metav1.ObjectMeta{Name: "extension-shoot-dns-service"}}

		rawExtension = &runtime.RawExtension{
			Raw: []byte(`{
  "apiVersion": "service.dns.extensions.gardener.cloud/v1alpha1",
  "kind": "DNSConfig",
  "dnsProviderReplication": {
	"enabled": true
  },
  "syncProvidersFromShootSpecDNS": true
}`),
		}
	)

	It("Create, Delete", Label("simple"), func() {
		ctx, cancel := context.WithTimeout(parentCtx, 15*time.Minute)
		defer cancel()

		By("Deploy Extension")
		Expect(execMake(ctx, "extension-up")).To(Succeed())

		By("Get Virtual Garden Client")
		gardenClientSet, err := kubernetes.NewClientFromSecret(ctx, runtimeClient, v1beta1constants.GardenNamespace, "gardener",
			kubernetes.WithDisabledCachedClient(),
			kubernetes.WithClientOptions(client.Options{Scheme: operatorclient.VirtualScheme}),
		)
		Expect(err).NotTo(HaveOccurred())

		By("Create workerless shoot")
		shoot := &gardencorev1beta1.Shoot{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "local-wl",
				Namespace: "garden-local",
			},
		}
		_, err = controllerutil.CreateOrUpdate(ctx, gardenClientSet.Client(), shoot, func() error {
			shoot.Spec.CloudProfile = &gardencorev1beta1.CloudProfileReference{
				Name: "local",
			}
			shoot.Spec.Region = "local"
			shoot.Spec.Provider = gardencorev1beta1.Provider{
				Type: "local",
			}
			shoot.Spec.Extensions = []gardencorev1beta1.Extension{
				{
					Type:           "shoot-dns-service",
					ProviderConfig: rawExtension,
				},
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Wait for Shoot to be in 'Processing' state >= 70%")
		waitForShootReconciliationToBeProcessing(ctx, gardenClientSet.Client(), shoot, 70)

		By("Patching external DNS provider")
		// as the dns-controller-manager cannot delete with provider "local", we patch it to "Ready"
		patchExternalProvider(ctx, client.ObjectKey{Namespace: "shoot--local--local-wl", Name: "external"})

		By("Wait for Shoot to be 'Ready'")
		waitForShootToBeReconciled(ctx, gardenClientSet.Client(), shoot)

		By("Check Operator Extension status")
		waitForOperatorExtensionToBeReconciled(ctx, operatorExtension)

		By("Check CRDs with no-cleanup label on shoot cluster")
		crdList := apiextensionsv1.CustomResourceDefinitionList{}
		shootClient, err := access.CreateShootClientFromAdminKubeconfig(ctx, gardenClientSet, shoot)
		Expect(err).NotTo(HaveOccurred())
		Expect(dnsv1alpha1.AddToScheme(shootClient.Client().Scheme())).To(Succeed())

		Expect(shootClient.Client().List(ctx, &crdList)).To(Succeed())
		var foundDNSEntryCRD, foundDNSProviderCRD, foundDNSAnnotationCRD bool
		for _, crd := range crdList.Items {
			if crd.Name == "dnsentries.dns.gardener.cloud" {
				foundDNSEntryCRD = true
				Expect(crd.Labels["shoot.gardener.cloud/no-cleanup"]).To(Equal("true"), "CRD dnsentries.dns.gardener.cloud should have label shoot.gardener.cloud/no-cleanup=true")
			}
			if crd.Name == "dnsproviders.dns.gardener.cloud" {
				foundDNSProviderCRD = true
				Expect(crd.Labels["shoot.gardener.cloud/no-cleanup"]).To(Equal("true"), "CRD dnsproviders.dns.gardener.cloud should have label shoot.gardener.cloud/no-cleanup=true")
			}
			if crd.Name == "dnsannotations.dns.gardener.cloud" {
				foundDNSAnnotationCRD = true
				Expect(crd.Labels["shoot.gardener.cloud/no-cleanup"]).To(Equal("true"), "CRD dnsannotations.dns.gardener.cloud should have label shoot.gardener.cloud/no-cleanup=true")
			}
		}
		Expect(foundDNSEntryCRD).To(BeTrue(), "CRD dnsentries.dns.gardener.cloud not found on shoot cluster")
		Expect(foundDNSProviderCRD).To(BeTrue(), "CRD dnsproviders.dns.gardener.cloud not found on shoot cluster")
		Expect(foundDNSAnnotationCRD).To(BeTrue(), "CRD dnsannotations.dns.gardener.cloud not found on shoot cluster")

		By("Check DNS provider replication")
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "knot-dns",
				Name:      "knot-dns-secret",
			},
		}
		Expect(runtimeClient.Get(ctx, client.ObjectKeyFromObject(secret), secret)).To(Succeed())
		providerSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "knot-dns-secret",
			},
		}
		_, err = controllerutil.CreateOrUpdate(ctx, shootClient.Client(), providerSecret, func() error {
			providerSecret.Data = secret.Data
			providerSecret.Type = secret.Type
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		provider := &dnsv1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "knot-dns",
			},
		}
		_, err = controllerutil.CreateOrUpdate(ctx, shootClient.Client(), provider, func() error {
			if provider.Annotations == nil {
				provider.Annotations = map[string]string{}
			}
			provider.Annotations["dns.gardener.cloud/class"] = "garden"
			provider.Spec.Type = "rfc2136"
			provider.Spec.SecretRef = &corev1.SecretReference{
				Name: "knot-dns-secret",
			}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Check DNSProvider has been successfully reconciled")
		waitForProviderReady(ctx, shootClient.Client(), provider, "shoot-dns-e2e-test.kind")

		By("Create shoot DNS entry")
		dnsEntry := &dnsv1alpha1.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: "default",
				Name:      "shoot-dns-entry",
			},
		}
		_, err = controllerutil.CreateOrUpdate(ctx, shootClient.Client(), dnsEntry, func() error {
			if dnsEntry.Annotations == nil {
				dnsEntry.Annotations = map[string]string{}
			}
			dnsEntry.Annotations["dns.gardener.cloud/class"] = "garden"
			dnsEntry.Spec.DNSName = "txt.shoot-dns-e2e-test.kind"
			dnsEntry.Spec.Targets = []string{"1.2.3.4"}
			return nil
		})
		Expect(err).NotTo(HaveOccurred())

		By("Check shoot DNS entry")
		waitForShootDNSEntryReady(ctx, shootClient.Client(), dnsEntry)

		By("Delete Shoot")
		deleteShoot(ctx, gardenClientSet.Client(), shoot)

		By("Wait for Shoot deletion")
		waitForShootToBeDeleted(ctx, gardenClientSet.Client(), shoot)
	})
})
