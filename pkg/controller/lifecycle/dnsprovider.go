// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package lifecycle

import (
	"context"
	"fmt"
	"time"

	dnsv1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/gardener/extensions/pkg/util"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/gardener/gardener/pkg/component"
	"github.com/gardener/gardener/pkg/controllerutils"
	"github.com/gardener/gardener/pkg/extensions"
	kutil "github.com/gardener/gardener/pkg/utils/kubernetes"
	"github.com/gardener/gardener/pkg/utils/retry"
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/helper"
)

// TimeNow returns the current time. Exposed for testing.
var TimeNow = time.Now

// NewProviderDeployWaiter creates a new instance of DeployWaiter for a specific DNSProvider.
func NewProviderDeployWaiter(
	logger logr.Logger,
	client client.Client,
	new *dnsv1alpha1.DNSProvider,
) component.DeployWaiter {
	return &provider{
		logger: logger,
		client: client,
		new:    new,

		dnsProvider: &dnsv1alpha1.DNSProvider{
			ObjectMeta: metav1.ObjectMeta{
				Name:      new.Name,
				Namespace: new.Namespace,
			},
		},
	}
}

type provider struct {
	logger logr.Logger
	client client.Client
	new    *dnsv1alpha1.DNSProvider

	dnsProvider *dnsv1alpha1.DNSProvider
}

func (p *provider) Deploy(ctx context.Context) error {
	_, err := controllerutils.GetAndCreateOrMergePatch(ctx, p.client, p.dnsProvider, func() error {
		p.dnsProvider.Labels = deepCopyMap(p.new.Labels)
		p.dnsProvider.Annotations = deepCopyMap(p.new.Annotations)
		metav1.SetMetaDataAnnotation(&p.dnsProvider.ObjectMeta, v1beta1constants.GardenerTimestamp, TimeNow().UTC().String())
		metav1.SetMetaDataAnnotation(&p.dnsProvider.ObjectMeta, ShootDNSServiceMaintainerAnnotation, "true")

		p.dnsProvider.Spec = *p.new.Spec.DeepCopy()
		return nil
	})
	return err
}

func (p *provider) Destroy(ctx context.Context) error {
	return client.IgnoreNotFound(p.client.Delete(ctx, p.dnsProvider))
}

func (p *provider) Wait(ctx context.Context) error {
	return extensions.WaitUntilObjectReadyWithHealthFunction(
		ctx,
		p.client,
		p.logger,
		CheckDNSProvider,
		p.dnsProvider,
		dnsv1alpha1.DNSProviderKind,
		5*time.Second,
		15*time.Second,
		2*time.Minute,
		nil,
	)
}

func (p *provider) WaitCleanup(ctx context.Context) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	return kutil.WaitUntilResourceDeleted(timeoutCtx, p.client, p.dnsProvider, 5*time.Second)
}

// CheckDNSProvider is similar to health.CheckExtensionObject, but implements the special handling for DNS providers
// as they don't implement extensionsv1alpha1.Object.
func CheckDNSProvider(obj client.Object) error {
	dnspr, ok := obj.(*dnsv1alpha1.DNSProvider)
	if !ok {
		return fmt.Errorf("object is no DNSProvider")
	}

	generation := dnspr.GetGeneration()
	observedGeneration := dnspr.Status.ObservedGeneration
	if observedGeneration != generation {
		return fmt.Errorf("observed generation outdated (%d/%d)", observedGeneration, generation)
	}

	if state := dnspr.Status.State; state != dnsv1alpha1.STATE_READY {
		var err error
		if msg := dnspr.Status.Message; msg != nil {
			err = fmt.Errorf("state %s: %s", state, *msg)
		} else {
			err = fmt.Errorf("state %s", state)
		}

		// TODO(timebertt): this should be the other way round: ErrorWithCodes should wrap the errorWithDNSState.
		// DetermineError first needs to be improved to properly wrap the given error, afterwards we can clean up this
		// code here
		if state == dnsv1alpha1.STATE_ERROR || state == dnsv1alpha1.STATE_INVALID {
			// return a retriable error for an Error or Invalid state (independent of the error code detection), which makes
			// WaitUntilObjectReadyWithHealthFunction not treat the error as severe immediately but still surface errors
			// faster, without retrying until the entire timeout is elapsed.
			// This is the same behavior as in other extension components which leverage health.CheckExtensionObject, where
			// ErrorWithCodes is returned if status.lastError is set (no matter if status.lastError.codes contains error codes).
			err = retry.RetriableError(util.DetermineError(err, helper.KnownCodes))
		}
		return &errorWithDNSState{underlying: err, state: state}
	}

	return nil
}

// ErrorWithDNSState is an error annotated with the state of a DNS object.
type ErrorWithDNSState interface {
	error

	// DNSState returns the state of the DNS object this error is about.
	DNSState() string
}

var _ ErrorWithDNSState = (*errorWithDNSState)(nil)

type errorWithDNSState struct {
	underlying error
	state      string
}

// Error returns the error message of the underlying (wrapped) error.
func (e *errorWithDNSState) Error() string {
	return e.underlying.Error()
}

// DNSState returns the state of the DNS object this error is about.
func (e *errorWithDNSState) DNSState() string {
	return e.state
}

// Unwrap returns the underlying (wrapped) error.
func (e *errorWithDNSState) Unwrap() error {
	return e.underlying
}

func deepCopyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	copy := map[string]string{}
	for k, v := range m {
		copy[k] = v
	}
	return copy
}
