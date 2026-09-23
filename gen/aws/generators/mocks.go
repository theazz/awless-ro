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

package main

import (
	"fmt"
	"strings"
	"text/template"

	"github.com/theazz/awless-ro/gen/aws"
)

// mockField is one canned response inside a service mock.
type mockField struct {
	Name string // Go field name, e.g. "instances"
	Type string // slice element type, e.g. "ec2types.Instance"
}

// mockMethod is one operation the mock answers, with the body precomputed:
// building it in Go keeps the template free of brace-escaping gymnastics.
type mockMethod struct {
	Name        string
	InputType   string
	OptionsType string
	OutputRef   string
	Body        string
}

type mockDef struct {
	Name        string // mockEc2
	Interface   string // awsfetch.Ec2API
	ManualState string // manualEc2Mock
	Fields      []mockField
	Methods     []mockMethod
}

func mockDefs() []mockDef {
	var defs []mockDef

	// Every service gets a mock, including the ones reached only through
	// hand-written fetchers: those have no generated methods, but the tests still
	// need something to stand in for the client.
	for _, api := range aws.UniqueApis() {
		pkg := aws.ApiPackage(api)
		def := mockDef{
			Name:        "mock" + aws.Title(api),
			Interface:   "awsfetch." + aws.ApiInterface(api),
			ManualState: "manual" + aws.Title(api) + "Mock",
		}

		for _, service := range aws.FetchersDefs {
			for _, f := range service.Fetchers {
				if f.Api != api || f.ManualFetcher || f.ApiMethod == "" {
					continue
				}

				field := mockFieldName(f.AWSType)
				def.Fields = append(def.Fields, mockField{Name: field, Type: f.AWSType})

				def.Methods = append(def.Methods, mockMethod{
					Name:        f.ApiMethod,
					InputType:   fmt.Sprintf("*%s.%sInput", pkg, f.ApiMethod),
					OptionsType: fmt.Sprintf("%s.Options", pkg),
					OutputRef:   fmt.Sprintf("%s.%sOutput", pkg, f.ApiMethod),
					Body:        mockOutputLiteral(pkg, f, field),
				})
			}
		}

		defs = append(defs, def)
	}

	return defs
}

// mockFieldName turns "ec2types.Instance" into "instances".
func mockFieldName(awsType string) string {
	name := awsType
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	return strings.ToLower(name) + "s"
}

// mockOutputLiteral builds the output the mock returns. Three shapes occur: the
// objects sit directly on the output, inside a list of containers, or behind a
// pointer to a wrapper shape.
func mockOutputLiteral(pkg string, f aws.Fetcher, field string) string {
	out := fmt.Sprintf("%s.%sOutput", pkg, f.ApiMethod)

	switch {
	case f.OutputsContainers != "":
		return fmt.Sprintf("&%s{%s: []%s{{%s: m.%s}}}",
			out, f.OutputsContainers, f.OutputsContainerType, f.OutputsExtractor, field)

	case strings.Contains(f.OutputsExtractor, "."):
		idx := strings.LastIndex(f.OutputsExtractor, ".")
		wrapperField, innerField := f.OutputsExtractor[:idx], f.OutputsExtractor[idx+1:]
		return fmt.Sprintf("&%s{%s: &%s{%s: m.%s}}",
			out, wrapperField, f.OutputsWrapperType, innerField, field)

	default:
		return fmt.Sprintf("&%s{%s: m.%s}", out, f.OutputsExtractor, field)
	}
}

func generateTestMocks() {
	templ, err := template.New("mocks").Parse(mocksTempl)
	if err != nil {
		panic(err)
	}

	var imports []importLine
	for _, api := range aws.GeneratedFetcherApis() {
		imports = append(imports,
			importLine{Path: aws.ApiImportPath(api)},
			importLine{Alias: aws.ApiTypesAlias(api), Path: aws.ApiImportPath(api) + "/types"},
		)
	}

	view := struct {
		Imports []importLine
		Mocks   []mockDef
	}{Imports: imports, Mocks: mockDefs()}

	writeTemplateToFile(templ, view, SERVICES_DIR, "gen_mocks_test.go")
}

const mocksTempl = `// Auto generated test mocks for the AWS cloud service

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

package awsservices

// DO NOT EDIT - This file was automatically generated with go generate

// Each mock embeds the narrow interface of its service rather than implementing
// it in full. That is what lets a mock answer only the operations a test cares
// about; anything else is a nil interface call, which panics with the operation
// name instead of quietly returning a zero value.
//
// Mocks hand back all their objects in a single page. Verifying that the
// paginators actually accumulate pages is a separate, hand-written test: the
// continuation token is named differently by each service, so generating
// multi-page mocks would encode more SDK trivia than it is worth.
//
// Each mock also embeds a manual<Api>Mock struct declared in mocks_test.go. That
// is where the canned answers for the hand-written fetchers live, since Go does
// not let one file add fields to a type declared in another.

import (
	"context"

{{- range $, $imp := .Imports }}
	{{ $imp.Alias }} "{{ $imp.Path }}"
{{- end }}

	"github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/cloud"
)

{{ range $, $mock := .Mocks }}
type {{ $mock.Name }} struct {
	{{ $mock.Interface }}
	{{ $mock.ManualState }}
{{- range $, $field := $mock.Fields }}
	{{ $field.Name }} []{{ $field.Type }}
{{- end }}
}

// cloud.Service, so that a mock can stand in for a whole service in the
// registry. The graph-building tests drive the fetchers directly, so these are
// inert on purpose.
func (m *{{ $mock.Name }}) Name() string            { return "" }
func (m *{{ $mock.Name }}) Region() string          { return "" }
func (m *{{ $mock.Name }}) Profile() string         { return "" }
func (m *{{ $mock.Name }}) ResourceTypes() []string { return []string{} }
func (m *{{ $mock.Name }}) IsSyncDisabled() bool    { return false }

func (m *{{ $mock.Name }}) Fetch(context.Context) (cloud.GraphAPI, error) {
	return nil, nil
}

func (m *{{ $mock.Name }}) FetchByType(context.Context, string) (cloud.GraphAPI, error) {
	return nil, nil
}

{{- range $, $method := $mock.Methods }}

func (m *{{ $mock.Name }}) {{ $method.Name }}(_ context.Context, _ {{ $method.InputType }}, _ ...func(*{{ $method.OptionsType }})) (*{{ $method.OutputRef }}, error) {
	return {{ $method.Body }}, nil
}
{{- end }}
{{ end }}`
