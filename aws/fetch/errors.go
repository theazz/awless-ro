/*
Copyright 2017 WALLIX

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package awsfetch

import (
	"errors"

	"github.com/aws/smithy-go"
)

// accessDeniedCodes are the error codes AWS services return when the caller's
// credentials are valid but lack permission. They are not consistent across
// services, which is why this is a set rather than a single comparison.
var accessDeniedCodes = map[string]bool{
	"AccessDenied":          true, // S3, and the literal code behind v1's "Access Denied" message
	"AccessDeniedException": true, // most JSON-protocol services
	"UnauthorizedOperation": true, // EC2
	"AuthorizationError":    true, // SNS
	"AuthFailure":           true, // EC2, invalid or unauthorised credentials
}

// IsAccessDenied reports whether err means the caller is not allowed to make the
// call. Syncing walks every service in parallel, so a single forbidden service
// has to be reportable as unavailable rather than failing the whole sync.
//
// Upstream matched on v1's awserr.RequestFailure and the message string
// "Access Denied". SDK v2 has no awserr, and matching a human-readable message
// was fragile anyway; error codes are part of the API contract.
func IsAccessDenied(err error) bool {
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return accessDeniedCodes[apiErr.ErrorCode()]
	}
	return false
}
