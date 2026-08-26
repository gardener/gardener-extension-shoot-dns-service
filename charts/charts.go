// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package charts

import (
	"embed"
)

// Internal contains the internal charts
//
//go:embed internal
var Internal embed.FS

// ChartsPath is the path to the charts
const ChartsPath = "internal"
