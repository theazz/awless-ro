package commands

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

// A command whose work fails must not have the flag list printed over the error.
//
// cobra prints usage whenever a command returns an error, which is right for a
// mistyped flag and wrong for "no image matched": the message a person needs scrolls
// off the top behind thirty lines of flags. The distinction is where in the run the
// failure happened, and the hooks run at exactly that boundary — after the command
// line has parsed, before the work starts.
func TestHooksSilenceUsageOnceTheCommandLineHasParsed(t *testing.T) {
	cmd := &cobra.Command{Use: "whatever"}
	if cmd.SilenceUsage {
		t.Fatal("cobra now defaults to silencing usage, so this hook has nothing to do")
	}

	applyHooks()(cmd, nil)

	if !cmd.SilenceUsage {
		t.Error("usage will be printed over a runtime error")
	}
}

// The hooks run in the order they are given, because later ones read what earlier ones
// set up: the AWS session has to exist before the syncer is built.
func TestHooksRunInOrder(t *testing.T) {
	var order []string
	note := func(name string) func(*cobra.Command, []string) error {
		return func(*cobra.Command, []string) error {
			order = append(order, name)
			return nil
		}
	}

	applyHooks(note("first"), note("second"), note("third"))(&cobra.Command{Use: "x"}, nil)

	want := []string{"first", "second", "third"}
	if len(order) != len(want) {
		t.Fatalf("ran %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Errorf("position %d ran %s, want %s", i, order[i], want[i])
		}
	}
}

// includeHookIf reads the flag when the hook runs, not when it is built, because the
// command line is parsed after the commands are wired together in init().
func TestIncludeHookIfReadsTheFlagAtRunTime(t *testing.T) {
	var enabled bool
	var ran int

	hook := includeHookIf(&enabled, func(*cobra.Command, []string) error {
		ran++
		return nil
	})
	cmd := &cobra.Command{Use: "x"}

	if err := hook(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if ran != 0 {
		t.Fatal("the hook ran while its flag was off")
	}

	enabled = true
	if err := hook(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if ran != 1 {
		t.Errorf("the hook ran %d times after its flag was turned on, want 1", ran)
	}
}

// An error from the wrapped hook is passed through rather than swallowed: applyHooks
// turns it into an exit, and the condition wrapper must not get in the way of that.
func TestIncludeHookIfPassesErrorsThrough(t *testing.T) {
	enabled := true
	boom := errors.New("no credentials")

	hook := includeHookIf(&enabled, func(*cobra.Command, []string) error { return boom })

	if err := hook(&cobra.Command{Use: "x"}, nil); !errors.Is(err, boom) {
		t.Errorf("got %v, want %v", err, boom)
	}
}
