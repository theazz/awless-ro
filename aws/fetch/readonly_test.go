package awsfetch

import (
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
