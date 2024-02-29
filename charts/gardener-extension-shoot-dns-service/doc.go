// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

//go:generate sh -c "bash $GARDENER_HACK_DIR/generate-controller-registration.sh extension-shoot-dns-service . $(cat ../../VERSION) ../../example/controller-registration.yaml Extension:shoot-dns-service"
//go:generate sh -c "sed -i 's/ type: shoot-dns-service/ type: shoot-dns-service\\n    workerlessSupported: true/' ../../example/controller-registration.yaml"

// Package chart enables go:generate support for generating the correct controller registration.
package chart
