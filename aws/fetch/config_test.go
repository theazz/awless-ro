package awsfetch

import (
	"reflect"
	"testing"
)

func TestNewConfigTakesClientsByName(t *testing.T) {
	conf := NewConfig(&AWSAPI{Iam: nil, Ec2: nil})
	if conf.APIs == nil {
		t.Fatal("APIs should never be nil")
	}
	if conf.Extra == nil {
		t.Fatal("Extra should never be nil")
	}
	if conf.Log == nil {
		t.Fatal("Log should never be nil")
	}

	if conf := NewConfig(nil); conf.APIs == nil {
		t.Fatal("NewConfig(nil) should still leave APIs usable")
	}
}

// TestNoAPIInterfaceIsEmpty guards the invariant that made the clients
// assignable by name in the first place.
//
// Upstream filled AWSAPI by reflection, dropping each client into the first
// field it was assignable to. That is safe only while no field's interface is
// empty, because an empty interface accepts every client and would swallow the
// first one offered, leaving the field that client belonged to nil. It is an
// easy state to reach by accident: a service awless-ro stops calling anything on
// loses both halves of its interface and becomes empty.
//
// The assignment is explicit now, so an empty interface no longer misfiles
// anything, but it still means a service is declared and never used - which is
// what actually happened with applicationautoscaling, left over from the deleted
// write commands.
func TestNoAPIInterfaceIsEmpty(t *testing.T) {
	apis := reflect.TypeOf(AWSAPI{})

	for i := 0; i < apis.NumField(); i++ {
		field := apis.Field(i)

		if field.Type.Kind() != reflect.Interface {
			t.Errorf("AWSAPI.%s is a %s, expected every field to be a narrow API interface",
				field.Name, field.Type.Kind())
			continue
		}
		if field.Type.NumMethod() == 0 {
			t.Errorf("AWSAPI.%s has an empty interface type %s: awless-ro calls no operation on that "+
				"service, so it should be dropped from gen/aws/fetchers_definitions.go",
				field.Name, field.Type)
		}
	}
}
