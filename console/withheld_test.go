package console

import (
	"bytes"
	"strings"
	"testing"

	"github.com/theazz/awless-ro/cloud/properties"
	"github.com/theazz/awless-ro/graph"
)

// A bootstrap script is a standard place to find tokens and passwords, so showing a
// resource must not print one unasked. It stays in the graph and stays available to a
// request that names it; it just does not land on screen by accident.
func TestUserDataIsWithheldFromTheResourceTable(t *testing.T) {
	const script = "#!/bin/bash\nexport DB_PASSWORD=hunter2\n"

	res := graph.InitResource("launchconfiguration", "lc-1")
	res.Properties()[properties.Name] = "web-launch"
	res.Properties()[properties.UserData] = script

	displayer, err := BuildOptions(
		WithColumnDefinitions(DefaultsColumnDefinitions["launchconfiguration"]),
		WithFormat("table"),
	).SetSource(res).Build()
	if err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := displayer.Print(&out); err != nil {
		t.Fatal(err)
	}
	printed := out.String()

	if strings.Contains(printed, "hunter2") {
		t.Errorf("the bootstrap script was printed:\n%s", printed)
	}
	if !strings.Contains(printed, "withheld") {
		t.Errorf("the property should be listed with a notice rather than dropped:\n%s", printed)
	}
	// The notice has to say how to get at it, or it is just a missing value.
	if !strings.Contains(printed, "--values-for") {
		t.Errorf("the notice should say how to see the value:\n%s", printed)
	}
	// Other properties are unaffected.
	if !strings.Contains(printed, "web-launch") {
		t.Errorf("an ordinary property went missing:\n%s", printed)
	}
}

// Withholding affects how the value is displayed, not whether it is there. The
// resource keeps it, which is what `show --values-for UserData` reads.
func TestWithholdingDoesNotRemoveTheProperty(t *testing.T) {
	const script = "#!/bin/bash\nexport DB_PASSWORD=hunter2\n"

	res := graph.InitResource("launchconfiguration", "lc-1")
	res.Properties()[properties.UserData] = script

	displayer, err := BuildOptions(
		WithColumnDefinitions(DefaultsColumnDefinitions["launchconfiguration"]),
		WithFormat("table"),
	).SetSource(res).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := displayer.Print(&bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}

	if got := res.Properties()[properties.UserData]; got != script {
		t.Errorf("the property was altered by displaying it: %v", got)
	}
}

// Displaying one resource ignores --format and always renders the property table, so
// there is exactly one way the value can be shown and it is the one being guarded.
// Recorded because the comment on withheldFromTable depends on it.
func TestSingleResourceDisplayIgnoresFormat(t *testing.T) {
	res := graph.InitResource("launchconfiguration", "lc-1")
	res.Properties()[properties.Name] = "web-launch"

	for _, format := range []string{"table", "json", "csv", "porcelain"} {
		displayer, err := BuildOptions(
			WithColumnDefinitions(DefaultsColumnDefinitions["launchconfiguration"]),
			WithFormat(format),
		).SetSource(res).Build()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := displayer.(*tableResourceDisplayer); !ok {
			t.Errorf("format %q on a single resource gave %T, expected the property table", format, displayer)
		}
	}
}
