// Copyright (c) 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helper

import (
	"regexp"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
)

var (
	unauthenticatedRegexp    = regexp.MustCompile(`(?i)(InvalidAuthenticationTokenTenant|Authentication failed|AuthFailure|invalid character|invalid_client|query returned no results|InvalidAccessKeyId|cannot fetch token|InvalidSecretAccessKey|InvalidSubscriptionId)`)
	unauthorizedRegexp       = regexp.MustCompile(`(?i)(Unauthorized|InvalidClientTokenId|SignatureDoesNotMatch|AuthorizationFailed|invalid_grant|Authorization Profile was not found|no active subscriptions|UnauthorizedOperation|not authorized|AccessDenied|OperationNotAllowed|Error 403|SERVICE_ACCOUNT_ACCESS_DENIED)`)
	quotaExceededRegexp      = regexp.MustCompile(`(?i)((?:^|[^t]|(?:[^s]|^)t|(?:[^e]|^)st|(?:[^u]|^)est|(?:[^q]|^)uest|(?:[^e]|^)quest|(?:[^r]|^)equest)LimitExceeded|Quotas|Quota.*exceeded|exceeded quota|Quota has been met|QUOTA_EXCEEDED|Maximum number of ports exceeded|ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS)`)
	rateLimitsExceededRegexp = regexp.MustCompile(`(?i)(RequestLimitExceeded|Throttling|Too many requests)`)

	// KnownCodes maps Gardener error codes to respective regex.
	KnownCodes = map[gardencorev1beta1.ErrorCode]func(string) bool{
		gardencorev1beta1.ErrorInfraUnauthenticated:    unauthenticatedRegexp.MatchString,
		gardencorev1beta1.ErrorInfraUnauthorized:       unauthorizedRegexp.MatchString,
		gardencorev1beta1.ErrorInfraQuotaExceeded:      quotaExceededRegexp.MatchString,
		gardencorev1beta1.ErrorInfraRateLimitsExceeded: rateLimitsExceededRegexp.MatchString,
	}
)
