// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	dnsapi "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/helper"
	wireapi "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/v1alpha1"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

////////////////////////////////////////////////////////////////////////////////
// state update handling

type StateHandler struct {
	*Env
	ctx      context.Context
	ext      *v1alpha1.Extension
	state    *apis.DNSState
	modified bool
	elem     *unstructured.Unstructured
	helper   *ShootDNSEntriesHelper
}

func NewStateHandler(ctx context.Context, env *Env, ext *v1alpha1.Extension, refresh bool) (*StateHandler, error) {
	var err error

	elem := &unstructured.Unstructured{}
	elem.SetAPIVersion(dnsapi.SchemeGroupVersion.String())
	elem.SetKind("DNSEntry")
	elem.SetNamespace(ext.Namespace)

	handler := &StateHandler{
		Env:    env,
		ctx:    ctx,
		ext:    ext,
		elem:   elem,
		helper: NewShootDNSEntriesHelper(ctx, env.Client(), ext),
	}
	handler.state, err = helper.GetExtensionState(ext)
	if err != nil || refresh {
		if err != nil {
			handler.modified = true
			handler.Infof("cannot setup state for %s -> refreshing: %s", ext.Name, err)
		} else {
			handler.Infof("refreshing state for %s", ext.Name)
		}
		_, err = handler.Refresh()
		if err != nil {
			handler.Infof("cannot setup state for %s -> refreshing: %s", ext.Name, err)
			return nil, err
		}
	}
	return handler, nil
}

func (s *StateHandler) Infof(msg string, args ...interface{}) {
	s.Info(fmt.Sprintf(msg, args...), "component", service.ServiceName, "namespace", s.ext.Namespace)
}

func (s *StateHandler) ShootDNSEntriesHelper() *ShootDNSEntriesHelper {
	return s.helper
}

func (s *StateHandler) Delete(name string) error {
	s.elem.SetName(name)
	if err := s.client.Delete(s.ctx, s.elem); client.IgnoreNotFound(err) != nil {
		return err
	}
	return nil
}

func (s *StateHandler) StateItems() []*apis.DNSEntry {
	return s.state.Entries
}

func (s *StateHandler) Refresh() (bool, error) {
	list, err := s.ShootDNSEntriesHelper().List()
	if err != nil {
		return false, err
	}
	/*
		list = append(list, dnsapi.DNSEntry{
			ObjectMeta: metav1.ObjectMeta{
				Name: "DUMMY",
			},
			Spec: dnsapi.DNSEntrySpec{
				DNSName: "bla.blub.de",
				Targets: []string{"8.8.8.8"},
			},
		})
	*/
	return s.EnsureEntries(list), nil
}

func (s *StateHandler) EnsureEntries(entries []dnsapi.DNSEntry) bool {
	mod := false
	names := sets.Set[string]{}
	for _, entry := range entries {
		mod = s.EnsureEntryFor(&entry) || mod
		names.Insert(entry.Name)
	}
	if len(entries) != len(s.state.Entries) {
		for i := len(s.state.Entries) - 1; i >= 0; i-- {
			e := s.state.Entries[i]
			if !names.Has(e.Name) {
				s.state.Entries = append(s.state.Entries[:i], s.state.Entries[i+1:]...)
				mod = true
			}
		}
	}
	s.modified = s.modified || mod
	return mod
}

func (s *StateHandler) EnsureEntryDeleted(name string) bool {
	for i, e := range s.state.Entries {
		if e.Name == name {
			s.state.Entries = append(s.state.Entries[:i], s.state.Entries[i+1:]...)
			s.modified = true
			return true
		}
	}
	return false
}

func (s *StateHandler) EnsureEntryFor(entry *dnsapi.DNSEntry) bool {
	for _, e := range s.state.Entries {
		if e.Name == entry.Name {
			mod := false
			if !reflect.DeepEqual(e.Spec, &entry.Spec) {
				mod = true
				e.Spec = entry.Spec.DeepCopy()
			}
			if !reflect.DeepEqual(&e.Annotations, &entry.Annotations) {
				mod = true
				e.Annotations = CopyMap(entry.Annotations)
			}
			if !reflect.DeepEqual(&e.Labels, &entry.Labels) {
				mod = true
				e.Labels = CopyMap(entry.Labels)
			}
			s.modified = s.modified || mod
			return mod
		}
	}

	e := &apis.DNSEntry{
		Name:        entry.Name,
		Labels:      CopyMap(entry.Labels),
		Annotations: CopyMap(entry.Annotations),
		Spec:        entry.Spec.DeepCopy(),
	}
	s.modified = true
	s.state.Entries = append(s.state.Entries, e)
	return true
}

func (s *StateHandler) Update(reason string) error {
	if s.modified {
		s.Info("updating modified state", "namespace", s.ext.Namespace, "extension", s.ext.Name, "reason", reason)
		wire := &wireapi.DNSState{}
		wire.APIVersion = wireapi.SchemeGroupVersion.String()
		wire.Kind = wireapi.DNSStateKind
		err := helper.Scheme.Convert(s.state, wire, nil)
		if err != nil {
			s.Infof("state conversion failed: %s", err)
			return err
		}
		if s.ext.Status.State == nil {
			s.ext.Status.State = &runtime.RawExtension{}
		}
		s.ext.Status.State.Raw, err = json.Marshal(wire)
		if err != nil {
			s.Info("marshalling failed", "error", err)
			return err
		}
		s.ext.Status.State.Object = nil
		err = s.client.Status().Update(s.ctx, s.ext)
		if err != nil {
			s.Info("update failed", "error", err)
		} else {
			s.modified = false
		}
		return err
	}
	return nil
}
