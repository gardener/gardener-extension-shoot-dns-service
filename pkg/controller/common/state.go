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
	"github.com/gardener/external-dns-management/pkg/dns"
	extapi "github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/sets"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/helper"
	wireapi "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/v1alpha1"
)

var (
	decoder runtime.Decoder
)

func init() {
	decoder = serializer.NewCodecFactory(helper.Scheme).UniversalDecoder()
}

func GetExtensionState(ext *extapi.Extension) (*apis.DNSState, error) {
	state := &apis.DNSState{}
	if ext.Status.State != nil && ext.Status.State.Raw != nil {
		data := ext.Status.State.Raw
		if LooksLikeCompressedEntriesState(data) {
			var err error
			data, err = DecompressEntriesState(data)
			if err != nil {
				return state, fmt.Errorf("could not decompress extension state: %w", err)
			}
		}
		if _, _, err := decoder.Decode(data, nil, state); err != nil {
			return state, fmt.Errorf("could not decode extension state: %w", err)
		}
	}
	return state, nil
}

////////////////////////////////////////////////////////////////////////////////
// state update handling

// StateHandler is a handler for the state of the extension.
type StateHandler struct {
	*Env
	ctx      context.Context
	ext      *extapi.Extension
	state    *apis.DNSState
	modified bool
	elem     *unstructured.Unstructured
	helper   *ShootDNSEntriesHelper
}

// NewStateHandler creates a new state handler.
func NewStateHandler(ctx context.Context, env *Env, ext *extapi.Extension) (*StateHandler, error) {
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
	handler.state, err = GetExtensionState(ext)
	return handler, err
}

// ShootDNSEntriesHelper returns the helper for the shoot DNSEntries.
func (s *StateHandler) ShootDNSEntriesHelper() *ShootDNSEntriesHelper {
	return s.helper
}

// StateItems returns the list of entries in the state.
func (s *StateHandler) StateItems() []*apis.DNSEntry {
	return s.state.Entries
}

// Refresh reads all entries from the control plane into the state.
func (s *StateHandler) Refresh() (bool, error) {
	list, err := s.ShootDNSEntriesHelper().List()
	if err != nil {
		return false, err
	}
	return s.EnsureEntries(list), nil
}

// DropAllEntries removes all entries from the state.
func (s *StateHandler) DropAllEntries() {
	if s.state == nil || !reflect.DeepEqual(s.state, &apis.DNSState{}) {
		s.Info("dropping all entries from state", "namespace", s.ext.Namespace)
		s.state = &apis.DNSState{}
		s.modified = true
	}
}

// EnsureEntries ensures that the entries in the state are up to date.
func (s *StateHandler) EnsureEntries(entries []dnsapi.DNSEntry) bool {
	mod := false
	names := sets.Set[string]{}
	for _, entry := range entries {
		mod = s.ensureEntryFor(&entry) || mod
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

func (s *StateHandler) copyRelevantAnnotations(entry *dnsapi.DNSEntry) map[string]string {
	annotations := CopyMap(entry.Annotations)
	delete(annotations, dns.AnnotationHardIgnore)
	return annotations
}

func (s *StateHandler) ensureEntryFor(entry *dnsapi.DNSEntry) bool {
	for _, e := range s.state.Entries {
		if e.Name == entry.Name {
			mod := false
			if !reflect.DeepEqual(e.Spec, &entry.Spec) {
				mod = true
				e.Spec = entry.Spec.DeepCopy()
			}
			annotations := s.copyRelevantAnnotations(entry)
			if !reflect.DeepEqual(e.Annotations, annotations) {
				mod = true
				e.Annotations = annotations
			}
			if !reflect.DeepEqual(e.Labels, entry.Labels) {
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
		Annotations: s.copyRelevantAnnotations(entry),
		Spec:        entry.Spec.DeepCopy(),
	}
	s.modified = true
	s.state.Entries = append(s.state.Entries, e)
	return true
}

// Update updates the state in the extension status.
func (s *StateHandler) Update(reason string) error {
	if s.modified || s.ext.Status.State == nil {
		s.Info("updating modified state", "namespace", s.ext.Namespace, "extension", s.ext.Name, "reason", reason)
		wire := &wireapi.DNSState{}
		wire.APIVersion = wireapi.SchemeGroupVersion.String()
		wire.Kind = wireapi.DNSStateKind
		err := helper.Scheme.Convert(s.state, wire, nil)
		if err != nil {
			s.Error(err, "state conversion failed")
			return err
		}
		if s.ext.Status.State == nil {
			s.ext.Status.State = &runtime.RawExtension{}
		}
		data, err := json.Marshal(wire)
		if err != nil {
			s.Error(err, "marshalling failed")
			return err
		}
		s.ext.Status.State.Raw, err = CompressEntriesState(data)
		if err != nil {
			s.Error(err, "compressing failed")
			return err
		}
		s.ext.Status.State.Object = nil
		err = s.client.Status().Update(s.ctx, s.ext)
		if err != nil {
			s.Error(err, "update failed")
			return err
		}
		s.modified = false
	}
	return nil
}
