// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package service

const (
	ExtensionType        = "shoot-dns-service"
	ServiceName          = ExtensionType
	ExtensionServiceName = "extension-" + ServiceName
	SeedChartName        = ServiceName + "-seed"
	ShootChartName       = ServiceName + "-shoot"

	// ImageName is the name of the dns controller manager.
	ImageName = "dns-controller-manager"

	// ShootAccessSecretName is the name of the shoot access secret in the seed.
	ShootAccessSecretName = "extension-shoot-dns-service"
	// ShootAccessServiceAccountName is the name of the service account used for accessing the shoot.
	ShootAccessServiceAccountName = ShootAccessSecretName
)
