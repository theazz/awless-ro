package config

import (
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestDefaults(t *testing.T) {
	f, e := os.MkdirTemp(".", "test")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(f)

	os.Setenv("__AWLESS_HOME", f)

	configDefinitions = map[string]*Definition{
		"aws.region":   {help: "AWS region", defaultValue: "eu-west-1"},
		"ec2.autosync": {help: "Auto sync AWS EC2", defaultValue: "true", parseParamFn: parseBool},
	}
	defaultsDefinitions = map[string]*Definition{
		"instance.type": {defaultValue: "t2.micro"},
	}

	t.Run("Config init", func(t *testing.T) {
		if err := InitConfig(map[string]string{}); err != nil {
			t.Fatal(err)
		}
		if err := LoadConfig(); err != nil {
			t.Fatal(err)
		}
		expect := map[string]interface{}{
			"aws.region":   "eu-west-1",
			"ec2.autosync": true,
		}
		if got, want := Config, expect; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		if got, want := Defaults, map[string]interface{}{"instance.type": "t2.micro"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("Set config without saving", func(t *testing.T) {
		if err := SetVolatile("aws.region", "us-west-1"); err != nil {
			t.Fatal(err)
		}
		expect := map[string]interface{}{"aws.region": "us-west-1", "ec2.autosync": true}
		if got, want := Config, expect; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}

		if err := LoadConfig(); err != nil {
			t.Fatal(err)
		}
		expect["aws.region"] = "eu-west-1"
		if got, want := Config, expect; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("Set config with saving", func(t *testing.T) {
		if err := Set("aws.region", "us-west-1"); err != nil {
			t.Fatal(err)
		}

		expect := map[string]interface{}{"aws.region": "us-west-1", "ec2.autosync": true}
		if got, want := Config, expect; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}

		if err := LoadConfig(); err != nil {
			t.Fatal(err)
		}
		if got, want := Config, expect; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("Set config with error", func(t *testing.T) {
		err := Set("ec2.autosync", "toto")
		if err == nil {
			t.Fatal("expect not nil error")
		}
		if got, want := err.Error(), "invalid value, expected a boolean, got 'toto'"; got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})

	t.Run("Set default", func(t *testing.T) {
		if err := Set("instance.type", "t2.nano"); err != nil {
			t.Fatal(err)
		}
		if got, want := Config, map[string]interface{}{"aws.region": "us-west-1", "ec2.autosync": true}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		if got, want := Defaults, map[string]interface{}{"instance.type": "t2.nano"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		if err := Set("instance.image", "ami-165a0876"); err != nil {
			t.Fatal(err)
		}
		if err := Set("instance.count", "1"); err != nil {
			t.Fatal(err)
		}
		if err := Set("subnet.create", "true"); err != nil {
			t.Fatal(err)
		}
		if got, want := Defaults, map[string]interface{}{"instance.type": "t2.nano", "instance.image": "ami-165a0876", "instance.count": 1, "subnet.create": true}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})

	t.Run("Display whole config", func(t *testing.T) {
		expect := `# Config parameters
   aws.region:     us-west-1   (string)   # AWS region
   ec2.autosync:   true        (bool)     # Auto sync AWS EC2

# Defaults
   ## Predefined
   instance.type:   t2.nano   (string)

   ## User defined
   instance.count:   1              (int)
   instance.image:   ami-165a0876   (string)
   subnet.create:    true           (bool)
`
		if got, want := DisplayConfig(), expect; got != want {
			t.Fatalf("got \n%s\nwant\n%s\n", got, want)
		}
	})

	t.Run("get default and config", func(t *testing.T) {
		v, ok := Get("aws.region")
		if got, want := ok, true; got != want {
			t.Fatalf("got %t want %t", got, want)
		}
		if got, want := fmt.Sprint(v), "us-west-1"; got != want {
			t.Fatalf("got %s want %s", got, want)
		}
		v, _ = Get("instance.image")
		if got, want := fmt.Sprint(v), "ami-165a0876"; got != want {
			t.Fatalf("got %s want %s", got, want)
		}
		_, ok = Get("not.here")
		if got, want := ok, false; got != want {
			t.Fatalf("got %t want %t", got, want)
		}
	})
	t.Run("unset default and config", func(t *testing.T) {
		if got, want := Config, map[string]interface{}{"aws.region": "us-west-1", "ec2.autosync": true}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		if got, want := Defaults, map[string]interface{}{"instance.type": "t2.nano", "instance.image": "ami-165a0876", "instance.count": 1, "subnet.create": true}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		Unset("ec2.autosync")
		Unset("instance.image")
		if got, want := Config, map[string]interface{}{"aws.region": "us-west-1"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		LoadConfig()
		if got, want := Config, map[string]interface{}{"aws.region": "us-west-1"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
		if got, want := Defaults, map[string]interface{}{"instance.type": "t2.nano", "instance.count": 1, "subnet.create": true}; !reflect.DeepEqual(got, want) {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	})
}

// A sync key added in a later release is missing from a config written by an
// older one. It must read as its declared default, not as on.
func TestMissingSyncKeyReadsAsItsDefault(t *testing.T) {
	f, e := os.MkdirTemp(".", "test")
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(f)
	os.Setenv("__AWLESS_HOME", f)

	saved := configDefinitions
	defer func() { configDefinitions = saved }()

	configDefinitions = map[string]*Definition{
		"aws.old.sync": {defaultValue: "true", parseParamFn: parseBool},
	}
	if err := InitConfig(map[string]string{}); err != nil {
		t.Fatal(err)
	}

	configDefinitions["aws.offbydefault.sync"] = &Definition{defaultValue: "false", parseParamFn: parseBool}
	configDefinitions["aws.onbydefault.sync"] = &Definition{defaultValue: "true", parseParamFn: parseBool}
	if err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
	expect := map[string]interface{}{"aws.old.sync": true, "aws.offbydefault.sync": false, "aws.onbydefault.sync": true}
	if got, want := Config, expect; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}

	if err := Set("aws.offbydefault.sync", "true"); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(); err != nil {
		t.Fatal(err)
	}
	if got, want := Config["aws.offbydefault.sync"], true; got != want {
		t.Fatalf("set value: got %#v, want %#v", got, want)
	}
}
