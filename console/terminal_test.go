package console

import "testing"

func TestTableWidthResolution(t *testing.T) {
	tty := func(w int) func() (int, bool) { return func() (int, bool) { return w, true } }
	notTTY := func() (int, bool) { return 0, false }

	cases := []struct {
		name        string
		explicit    int
		explicitSet bool
		terminal    func() (int, bool)
		want        int
	}{
		{"terminal, no flag: the terminal width", 0, false, tty(218), 218},
		{"pipe, no flag: no limit", 0, false, notTTY, 0},
		{"terminal reporting no width: no limit", 0, false, tty(0), 0},
		{"pipe, --max-width 80", 80, true, notTTY, 80},
		{"terminal, --max-width 80 wins", 80, true, tty(218), 80},
		{"terminal, --max-width 0: no limit", 0, true, tty(218), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TableWidth(tc.explicit, tc.explicitSet, tc.terminal); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}

	t.Run("COLUMNS is ignored", func(t *testing.T) {
		t.Setenv("COLUMNS", "40")
		if got := TableWidth(0, false, notTTY); got != 0 {
			t.Fatalf("pipe with COLUMNS=40: got %d, want 0", got)
		}
		if got := TableWidth(0, false, tty(218)); got != 218 {
			t.Fatalf("terminal with COLUMNS=40: got %d, want 218", got)
		}
	})
}
