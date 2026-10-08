package commands

import "testing"

func TestMaxWidthFlagParsing(t *testing.T) {
	if got := (&maxWidthValue{}).String(); got != "" {
		t.Errorf("an unset flag reads %q, want empty so the help shows no default", got)
	}

	for _, tc := range []struct {
		in   string
		want int
	}{
		{"80", 80},
		{"0", 0},
		{"5", 5},
	} {
		v := &maxWidthValue{}
		if err := v.Set(tc.in); err != nil {
			t.Errorf("Set(%q): %v", tc.in, err)
			continue
		}
		if v.n != tc.want || !v.set {
			t.Errorf("Set(%q) = {%d, %t}, want {%d, true}", tc.in, v.n, v.set, tc.want)
		}
		if v.String() != tc.in {
			t.Errorf("Set(%q) then String() = %q", tc.in, v.String())
		}
	}

	for _, in := range []string{"-1", "abc", "", "1.5", "80px", " 80"} {
		v := &maxWidthValue{}
		if err := v.Set(in); err == nil {
			t.Errorf("Set(%q) accepted, got width %d", in, v.n)
		}
		if v.set {
			t.Errorf("Set(%q) failed but marked the flag as given", in)
		}
	}
}

// The flag belongs to the commands that print a table. A global one would be accepted
// by sync, whoami and the rest and silently do nothing.
func TestMaxWidthFlagIsOnTableCommandsOnly(t *testing.T) {
	if listCmd.PersistentFlags().Lookup("max-width") == nil {
		t.Error("list has no --max-width for its subcommands to inherit")
	}
	if showCmd.Flags().Lookup("max-width") == nil {
		t.Error("show has no --max-width")
	}
	if RootCmd.PersistentFlags().Lookup("max-width") != nil {
		t.Error("--max-width is global")
	}
	for _, cmd := range []struct {
		name  string
		found bool
	}{
		{"sync", syncCmd.Flags().Lookup("max-width") != nil},
		{"whoami", whoamiCmd.Flags().Lookup("max-width") != nil},
		{"inspect", inspectCmd.Flags().Lookup("max-width") != nil},
		{"ssh", sshCmd.Flags().Lookup("max-width") != nil},
	} {
		if cmd.found {
			t.Errorf("%s takes --max-width but prints no table", cmd.name)
		}
	}
}
