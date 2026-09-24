package awsfetch

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/smithy-go"
)

type apiError struct {
	code string
}

func (e apiError) Error() string        { return "api error " + e.code }
func (e apiError) ErrorCode() string    { return e.code }
func (e apiError) ErrorMessage() string { return e.code }
func (e apiError) ErrorFault() smithy.ErrorFault {
	return smithy.FaultClient
}

// Access-denied codes are not consistent across AWS services, which is why this is a
// set and not one comparison. Getting it wrong has a specific consequence: syncing
// walks every service in parallel, and a service the caller cannot read has to be
// reportable as unavailable rather than failing the whole sync.
func TestIsAccessDeniedRecognisesEveryServiceSpelling(t *testing.T) {
	denied := []string{
		"AccessDenied",          // S3
		"AccessDeniedException", // most JSON-protocol services
		"UnauthorizedOperation", // EC2
		"AuthorizationError",    // SNS
		"AuthFailure",           // EC2, credentials rejected
	}

	for _, code := range denied {
		if !IsAccessDenied(apiError{code: code}) {
			t.Errorf("code %q was not recognised as access denied; the service would fail the sync instead of being marked unavailable", code)
		}
	}
}

func TestIsAccessDeniedIgnoresOtherFailures(t *testing.T) {
	other := []error{
		nil,
		errors.New("connection reset"),
		apiError{code: "Throttling"},
		apiError{code: "RequestLimitExceeded"},
		apiError{code: "InvalidParameterValue"},
		// A code that merely contains one of ours must not match.
		apiError{code: "NotAccessDenied"},
	}

	for _, err := range other {
		if IsAccessDenied(err) {
			t.Errorf("%v was mistaken for an access denial, which would hide a real failure behind a permissions message", err)
		}
	}
}

// The error is usually wrapped by the time it gets here: the SDK puts it inside an
// operation error, and the fetch engine collects it into its own. Matching has to see
// through both, which is why it uses errors.As rather than a type assertion.
func TestIsAccessDeniedSeesThroughWrapping(t *testing.T) {
	inner := apiError{code: "UnauthorizedOperation"}

	wrapped := []error{
		fmt.Errorf("fetching instances: %w", inner),
		fmt.Errorf("outer: %w", fmt.Errorf("middle: %w", inner)),
		&smithy.OperationError{ServiceID: "EC2", OperationName: "DescribeInstances", Err: inner},
	}

	for _, err := range wrapped {
		if !IsAccessDenied(err) {
			t.Errorf("wrapped error was not recognised: %v", err)
		}
	}
}
