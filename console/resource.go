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

package console

import (
	"fmt"
	"io"

	"github.com/olekukonko/tablewriter"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/cloud/properties"
)

type tableResourceDisplayer struct {
	maxwidth          int
	r                 cloud.Resource
	columnDefinitions []ColumnDefinition
}

// withheldFromTable lists properties whose value is not printed in the property table
// that `show` prints by default.
//
// UserData is a launch configuration's bootstrap script, and bootstrap scripts are a
// standard place to find tokens, registry credentials and database passwords. Printing
// one unasked puts it on screen, into the scrollback and into whatever recorded the
// session. It is also base64 and frequently kilobytes long, so it wrecks a two-column
// table either way.
//
// Nothing is hidden from a request that names the property: `show --values-for
// UserData` prints it in full, and it is stored in the graph as before. The point is
// only that showing a resource should not spray a secret.
//
// That single escape hatch is the only one, because displaying one resource ignores
// --format and always comes through here. That is a separate oddity, not something
// this relies on.
var withheldFromTable = map[string]bool{
	properties.UserData: true,
}

func withheldNotice(prop string, value interface{}) string {
	return fmt.Sprintf("<%d bytes withheld, see `--values-for %s`>", len(fmt.Sprint(value)), prop)
}

func (d *tableResourceDisplayer) Print(w io.Writer) error {
	values := make(table, len(d.r.Properties()))

	i := 0
	propertyNameMaxWith := 13
	for prop, val := range d.r.Properties() {
		var header ColumnDefinition
		for _, h := range d.columnDefinitions {
			if h.propKey() == prop {
				header = h
			}
		}
		if header == nil {
			header = &StringColumnDefinition{Prop: prop}
		}

		if v := values[i]; v == nil {
			values[i] = make([]interface{}, 2)
		}
		values[i][0] = header.title()
		if l := len(header.title()); l > propertyNameMaxWith {
			propertyNameMaxWith = l
		}
		if withheldFromTable[prop] {
			values[i][1] = withheldNotice(prop, val)
		} else {
			values[i][1] = header.format(val)
		}
		i++
	}

	ds := defaultSorter{sortBy: []int{0}}
	ds.sort(values)

	valueColumnMaxwidth := d.maxwidth - (propertyNameMaxWith + 7) // ( = border + 2 * margin + border + 2 * margin + border)
	if valueColumnMaxwidth <= 0 {
		valueColumnMaxwidth = 50
	}

	table := tablewriter.NewWriter(w)
	table.SetBorders(tablewriter.Border{Left: true, Top: false, Right: true, Bottom: false})
	table.SetColWidth(valueColumnMaxwidth)
	table.SetCenterSeparator("|")
	table.SetAlignment(tablewriter.ALIGN_LEFT)
	table.SetHeader([]string{"Property" + ds.symbol(), "Value"})

	wraper := autoWraper{maxWidth: valueColumnMaxwidth, wrappingChar: " "}

	for i := range values {
		if val := fmt.Sprint(values[i][1]); val != "" {
			table.Append([]string{fmt.Sprint(values[i][0]), wraper.Wrap(val)})
		}
	}

	table.Render()

	return nil
}

func (d *tableResourceDisplayer) SetResource(r cloud.Resource) {
	d.r = r
}
