package awsfetch

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// awless-ro is read-only, and this is the test that makes that a fact rather than an
// intention.
//
// Every AWS call the tool can make goes through one of the narrow interfaces on AWSAPI:
// nothing else holds a concrete SDK client, so nothing else can reach the API. That
// makes the set of methods on those interfaces the complete list of operations the
// binary is capable of. If every one of them is a read, the tool cannot change anything
// in an account, whatever permissions its credentials happen to carry — and in practice
// they often carry a great deal, because people point this at accounts they administer.
//
// The check reflects over AWSAPI rather than over a list written by hand, so a service
// added to the struct is covered without anyone remembering to add it here.
func TestEveryAWSOperationIsARead(t *testing.T) {
	// AWS names read operations with one of these. Anything else — Create, Delete,
	// Put, Update, Modify, Run, Terminate, Attach, Detach, Tag, Reboot, Start, Stop,
	// Copy, Import, Register, Associate, Enable, Authorize, Revoke — changes
	// something.
	readPrefixes := []string{"Describe", "Get", "List", "Head"}

	apis := reflect.TypeOf(AWSAPI{})
	if apis.NumField() == 0 {
		t.Fatal("AWSAPI has no fields; this test would pass by vacuum")
	}

	total := 0
	for i := range apis.NumField() {
		field := apis.Field(i)
		iface := field.Type

		if iface.Kind() != reflect.Interface {
			t.Errorf("AWSAPI.%s is a %s, not an interface: a concrete client here would "+
				"escape this check entirely", field.Name, iface.Kind())
			continue
		}
		if iface.NumMethod() == 0 {
			t.Errorf("AWSAPI.%s is an empty interface, which every client satisfies; "+
				"see the comment on NewConfig", field.Name)
			continue
		}

		for m := range iface.NumMethod() {
			method := iface.Method(m)
			total++

			isRead := false
			for _, prefix := range readPrefixes {
				if strings.HasPrefix(method.Name, prefix) {
					isRead = true
					break
				}
			}
			if !isRead {
				t.Errorf("AWSAPI.%s can call %s, which is not a read operation.\n"+
					"awless-ro must not be able to change anything in an AWS account. If this "+
					"really is a read, add its prefix to readPrefixes and say why.",
					field.Name, method.Name)
			}
		}
	}

	// A floor, so that the test failing to see any methods at all would be noticed.
	if total < 50 {
		t.Errorf("only %d operations were inspected, which is too few to be the whole surface", total)
	}
	if !t.Failed() {
		t.Logf("%d operations across %d services, all reads", total, apis.NumField())
	}
}

// The interfaces are the only route to AWS. If a fetcher held a concrete SDK client it
// could call anything, so the fields are checked for being interfaces above; this
// asserts the struct is the single place clients live.
func TestAWSAPICoversEveryServiceField(t *testing.T) {
	apis := reflect.TypeOf(AWSAPI{})

	for i := range apis.NumField() {
		field := apis.Field(i)
		if !field.IsExported() {
			t.Errorf("AWSAPI.%s is unexported; every client has to be nameable by callers", field.Name)
		}
		if !strings.HasSuffix(field.Type.Name(), "API") {
			t.Errorf("AWSAPI.%s has type %s, expected one of the narrow XxxAPI interfaces",
				field.Name, field.Type)
		}
	}
}

// Read-only is not the same as harmless. Some reads return a secret or a credential
// itself rather than metadata about it, and TestEveryAWSOperationIsARead lets them
// through because their names start with Get. awless-ro lists what exists; it never
// needs the value of a secret, a decrypted parameter or a fresh token, so none of
// these may appear on the API surface.
func TestNoOperationReturnsSecretsOrCredentials(t *testing.T) {
	for _, found := range secretReads(reflect.TypeOf(AWSAPI{})) {
		t.Errorf("%s\nawless-ro shows metadata, never secret values or credentials.", found)
	}
}

// The check itself must catch a denied operation, or the test above passes by vacuum.
// It runs on a stand-in struct: adding the method to a real API interface would break
// the build (clients and mocks must implement it) before the test ever ran.
func TestSecretReadsCatchesDeniedOperations(t *testing.T) {
	type secretsAPI interface {
		ListSecrets()
		GetSecretValue()
	}
	type ssmAPI interface {
		DescribeParameters()
		GetParametersByPath()
	}
	type fakeAPIs struct {
		Secretsmanager secretsAPI
		Ssm            ssmAPI
	}
	found := secretReads(reflect.TypeOf(fakeAPIs{}))
	if len(found) != 2 {
		t.Fatalf("want GetSecretValue and GetParametersByPath reported, got %d: %q", len(found), found)
	}
	for i, want := range []string{"AWSAPI.Secretsmanager can call GetSecretValue", "AWSAPI.Ssm can call GetParametersByPath"} {
		if !strings.HasPrefix(found[i], want) {
			t.Errorf("report %d = %q, want it to start with %q", i, found[i], want)
		}
	}
}

// secretReads lists, for every interface field of apis, the operations that return a
// secret or a credential.
func secretReads(apis reflect.Type) []string {
	denied := map[string]string{
		"GetSecretValue":            "Secrets Manager: returns the secret value; use ListSecrets / DescribeSecret",
		"BatchGetSecretValue":       "Secrets Manager: returns secret values",
		"GetParameter":              "SSM: returns the parameter value, decrypted on request; use DescribeParameters",
		"GetParameters":             "SSM: returns parameter values",
		"GetParametersByPath":       "SSM: returns parameter values",
		"GetParameterHistory":       "SSM: returns past parameter values",
		"GetPasswordData":           "EC2: returns the encrypted Windows administrator password",
		"GetAuthorizationToken":     "ECR: returns a registry login token",
		"GetSessionToken":           "STS: issues temporary credentials",
		"GetFederationToken":        "STS: issues temporary credentials",
		"GetRoleCredentials":        "SSO: issues role credentials",
		"GetCredentialsForIdentity": "Cognito: issues credentials",
	}

	var found []string
	for i := range apis.NumField() {
		field := apis.Field(i)
		if field.Type.Kind() != reflect.Interface {
			continue // reported by TestEveryAWSOperationIsARead
		}
		for m := range field.Type.NumMethod() {
			name := field.Type.Method(m).Name
			if why, ok := denied[name]; ok {
				found = append(found, fmt.Sprintf("AWSAPI.%s can call %s (%s).", field.Name, name, why))
			}
		}
	}
	return found
}
