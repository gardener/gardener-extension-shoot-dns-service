/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 */

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/gardener/gardener/extensions/pkg/controller"
	"github.com/gardener/gardener/pkg/apis/extensions/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	dnsapi "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"

	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/helper"
	wireapi "github.com/gardener/gardener-extension-shoot-dns-service/pkg/apis/v1alpha1"
	"github.com/gardener/gardener-extension-shoot-dns-service/pkg/service"
)

////////////////////////////////////////////////////////////////////////////////
// state update handling

type StateHandler struct {
	*Env
	ext      *v1alpha1.Extension
	state    *apis.DNSState
	modified bool
	elem     *unstructured.Unstructured
}

func NewStateHandler(ctx context.Context, env *Env, ext *v1alpha1.Extension, refresh bool) (*StateHandler, error) {
	var err error

	elem := &unstructured.Unstructured{}
	elem.SetAPIVersion(dnsapi.SchemeGroupVersion.String())
	elem.SetKind("DNSEntry")
	elem.SetNamespace(ext.Namespace)

	handler := &StateHandler{
		Env:  env,
		ext:  ext,
		elem: elem,
	}
	handler.state, err = helper.GetExtensionState(ext)
	if err != nil || refresh {
		if err != nil {
			handler.modified = true
			handler.Infof("cannot setup state for %s -> refreshing: %s", ext.Name, err)
		} else {
			handler.Infof("refreshing state for %s", ext.Name)
		}
		cluster, err := controller.GetCluster(ctx, env.Client(), ext.Namespace)
		if err != nil {
			return nil, err
		}
		_, err = handler.Refresh(cluster)
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

func (s *StateHandler) ShootID(cluster *controller.Cluster) (string, string, error) {
	if cluster.Shoot.Status.ClusterIdentity == nil {
		return "", "", fmt.Errorf("missing shoot cluster identity")
	}
	return *cluster.Shoot.Status.ClusterIdentity, ShortenID(*cluster.Shoot.Status.ClusterIdentity, 63), nil
}

func (s *StateHandler) List(cluster *controller.Cluster) ([]dnsapi.DNSEntry, error) {
	_, labelValue, err := s.ShootID(cluster)
	if err != nil {
		return nil, err
	}
	list := &dnsapi.DNSEntryList{}
	err = s.ListObjects(list, client.InNamespace(s.ext.Namespace), client.MatchingLabels(map[string]string{s.EntryLabel(): labelValue}))
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	return list.Items, nil
}

func (s *StateHandler) Delete(name string) error {
	s.elem.SetName(name)
	if err := s.client.Delete(s.ctx, s.elem); client.IgnoreNotFound(err) != nil {
		return err
	}
	return nil
}

func (s *StateHandler) Items() []*apis.DNSEntry {
	return s.state.Entries
}

func (s *StateHandler) Refresh(cluster *controller.Cluster) (bool, error) {
	list, err := s.List(cluster)
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
	names := sets.String{}
	for _, entry := range entries {
		mod = s.EnsureEntryFor(&entry)
		names.Insert(entry.Name)
	}
	if len(entries) != len(s.state.Entries) {
		for i, e := range s.state.Entries {
			if !names.Has(e.Name) {
				mod = true
				s.state.Entries = append(s.state.Entries[:i], s.state.Entries[i+1:]...)
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

func (s *StateHandler) Update() error {
	if s.modified {
		s.Infof("updating modified state for %s", s.ext.Name)
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
		s.ext.Status.State.Object = nil
		err = s.client.Status().Update(s.ctx, s.ext)
		if err != nil {
			s.Infof("update failed: %s", err)
		} else {
			s.modified = false
		}
		return err
	}
	return nil
}
