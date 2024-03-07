// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"fmt"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	extensionsv1alpha1 "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/utils/retry"
	retryfake "github.com/gardener/gardener/pkg/utils/retry/fake"
	"github.com/gardener/gardener/pkg/utils/test"
	. "github.com/gardener/gardener/pkg/utils/test/matchers"
	mockclient "github.com/gardener/gardener/third_party/mock/controller-runtime/client"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var _ = Describe("#DNSProvider", func() {
	const (
		deployNS        = "test-chart-namespace"
		secretName      = "extensions-dns-test-deploy"
		dnsProviderName = "test-deploy"
	)

	var (
		ctrl      *gomock.Controller
		ctx       context.Context
		c         client.Client
		scheme    *runtime.Scheme
		fakeOps   *retryfake.Ops
		now       time.Time
		resetVars func()

		expected         *dnsv1alpha1.DNSProvider
		vals             *dnsv1alpha1.DNSProvider
		log              logr.Logger
		defaultDepWaiter component.DeployWaiter
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())

		now = time.Now()
		fakeOps = &retryfake.Ops{MaxAttempts: 1}
		resetVars = test.WithVars(
			&retry.Until, fakeOps.Until,
			&retry.UntilTimeout, fakeOps.UntilTimeout,
			&TimeNow, func() time.Time { return now },
		)

		ctx = context.TODO()
		log = logf.Log.WithName("test")

		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).NotTo(HaveOccurred())
		Expect(dnsv1alpha1.AddToScheme(scheme)).NotTo(HaveOccurred())

		c = fake.NewClientBuilder().WithScheme(scheme).Build()

		vals = &dnsv1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dnsProviderName,
				Namespace: deployNS,
			},
			Spec: dnsv1alpha1.DNSProviderSpec{
				Type:      "some-emptyProvider",
				SecretRef: &corev1.SecretReference{Name: secretName},
				Domains: &dnsv1alpha1.DNSSelection{
					Include: []string{"foo.com"},
					Exclude: []string{"baz.com"},
				},
				Zones: &dnsv1alpha1.DNSSelection{
					Include: []string{"goo.local"},
					Exclude: []string{"dodo.local"},
				},
			},
		}

		expected = &dnsv1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      dnsProviderName,
				Namespace: deployNS,
				Annotations: map[string]string{
					"gardener.cloud/timestamp": now.UTC().String(),
				},
			},
			Spec: dnsv1alpha1.DNSProviderSpec{
				Type: "some-emptyProvider",
				SecretRef: &corev1.SecretReference{
					Name: secretName,
				},
				Domains: &dnsv1alpha1.DNSSelection{
					Include: []string{"foo.com"},
					Exclude: []string{"baz.com"},
				},
				Zones: &dnsv1alpha1.DNSSelection{
					Include: []string{"goo.local"},
					Exclude: []string{"dodo.local"},
				},
			},
		}

		defaultDepWaiter = NewProviderDeployWaiter(log, c, vals)
	})

	AfterEach(func() {
		resetVars()
		ctrl.Finish()
	})

	Describe("#Deploy", func() {
		DescribeTable("correct DNSProvider is created",
			func(mutator func()) {
				mutator()

				Expect(defaultDepWaiter.Deploy(ctx)).ToNot(HaveOccurred())

				actual := &dnsv1alpha1.DNSProvider{}
				err := c.Get(ctx, client.ObjectKey{Name: dnsProviderName, Namespace: deployNS}, actual)

				Expect(err).NotTo(HaveOccurred())
				Expect(actual).To(DeepDerivativeEqual(expected))
			},

			Entry("with no modification", func() {}),
			Entry("with no domains", func() {
				vals.Spec.Domains = nil
				expected.Spec.Domains = nil
			}),
			Entry("with no exclude domain", func() {
				vals.Spec.Domains.Exclude = nil
				expected.Spec.Domains.Exclude = nil
			}),
			Entry("with no zones", func() {
				vals.Spec.Zones = nil
				expected.Spec.Zones = nil
			}),
			Entry("with custom labels", func() {
				vals.Labels = map[string]string{"foo": "bar"}
				expected.ObjectMeta.Labels = map[string]string{"foo": "bar"}
			}),
			Entry("with custom annotations", func() {
				vals.Annotations = map[string]string{"foo": "bar"}
				expected.ObjectMeta.Annotations = map[string]string{"foo": "bar"}
			}),
			Entry("with no exclude zones", func() {
				vals.Spec.Zones.Exclude = nil
				expected.Spec.Zones.Exclude = nil
			}),
		)
	})

	Describe("#Destroy", func() {
		It("should not return error when it's not found", func() {
			Expect(defaultDepWaiter.Destroy(ctx)).ToNot(HaveOccurred())
		})

		It("should not return error when it's deleted successfully", func() {
			Expect(c.Create(ctx, expected)).ToNot(HaveOccurred(), "adding pre-existing emptyEntry succeeds")

			Expect(defaultDepWaiter.Destroy(ctx)).ToNot(HaveOccurred())
		})

		It("should not return error when it's deleted successfully", func() {
			mc := mockclient.NewMockClient(ctrl)
			mc.EXPECT().Delete(ctx, &dnsv1alpha1.DNSProvider{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dnsProviderName,
					Namespace: deployNS,
				}}).Times(1).Return(fmt.Errorf("some random error"))

			Expect(NewProviderDeployWaiter(log, mc, vals).Destroy(ctx)).To(HaveOccurred())
		})
	})

	Describe("#Wait", func() {
		It("should return error when it's not found", func() {
			Expect(defaultDepWaiter.Wait(ctx)).To(HaveOccurred())
		})

		It("should retry getting object if it does not exist in the cache yet", func() {
			mc := mockclient.NewMockClient(ctrl)
			mc.EXPECT().Scheme().Return(scheme).AnyTimes()

			expected.Status.State = "Ready"
			gomock.InOrder(
				mc.EXPECT().Get(gomock.Any(), client.ObjectKeyFromObject(expected), gomock.AssignableToTypeOf(expected)).
					Return(apierrors.NewNotFound(extensionsv1alpha1.Resource("dnsproviders"), expected.Name)),
				mc.EXPECT().Get(gomock.Any(), client.ObjectKeyFromObject(expected), gomock.AssignableToTypeOf(expected)).
					DoAndReturn(func(ctx context.Context, key client.ObjectKey, obj *dnsv1alpha1.DNSProvider, opts ...client.GetOption) error {
						expected.DeepCopyInto(obj)
						return nil
					}),
			)

			fakeOps.MaxAttempts = 2
			defaultDepWaiter = NewProviderDeployWaiter(log, mc, vals)
			Expect(defaultDepWaiter.Wait(ctx)).To(Succeed())
		})

		It("should return error when it's not ready", func() {
			expected.Status.State = "dummy-not-ready"
			expected.Status.Message = ptr.To("some-error-message")

			Expect(c.Create(ctx, expected)).ToNot(HaveOccurred(), "adding pre-existing emptyProvider succeeds")

			Expect(defaultDepWaiter.Wait(ctx)).To(HaveOccurred())
		})

		It("should return error if we haven't observed the latest timestamp annotation", func() {
			By("deploy")
			// Deploy should fill internal state with the added timestamp annotation
			Expect(defaultDepWaiter.Deploy(ctx)).To(Succeed())

			By("patch object")
			patch := client.MergeFrom(expected.DeepCopy())
			expected.Status.State = "Ready"
			// add old timestamp annotation
			expected.ObjectMeta.Annotations = map[string]string{
				v1beta1constants.GardenerTimestamp: now.Add(-time.Millisecond).UTC().String(),
			}
			Expect(c.Patch(ctx, expected, patch)).To(Succeed(), "patching dnsprovider succeeds")

			By("wait")
			Expect(defaultDepWaiter.Wait(ctx)).NotTo(Succeed(), "dnsprovider indicates error")
		})

		It("should return no error when it's ready", func() {
			By("deploy")
			// Deploy should fill internal state with the added timestamp annotation
			Expect(defaultDepWaiter.Deploy(ctx)).To(Succeed())

			By("patch object")
			patch := client.MergeFrom(expected.DeepCopy())
			expected.Status.State = "Ready"
			// add up-to-date timestamp annotation
			expected.ObjectMeta.Annotations = map[string]string{
				v1beta1constants.GardenerTimestamp: now.UTC().String(),
			}
			Expect(c.Patch(ctx, expected, patch)).To(Succeed(), "patching dnsprovider succeeds")

			By("wait")
			Expect(defaultDepWaiter.Wait(ctx)).To(Succeed(), "dnsprovider is ready")
		})
	})

	Describe("#WaitCleanup", func() {
		It("should not return error when it's already removed", func() {
			Expect(defaultDepWaiter.WaitCleanup(ctx)).ToNot(HaveOccurred())
		})
	})
})
