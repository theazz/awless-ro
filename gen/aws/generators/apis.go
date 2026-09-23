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
	"text/template"

	"github.com/theazz/awless-ro/gen/aws"
)

// apiDef is the template view of one AWS service: which narrow interface to
// declare for it and which operations the generated fetchers call on it.
type apiDef struct {
	Key        string
	Package    string
	ImportPath string
	Interface  string
	Methods    []string
}

func apiDefs() []apiDef {
	var defs []apiDef
	for _, key := range aws.UniqueApis() {
		defs = append(defs, apiDef{
			Key:        key,
			Package:    aws.ApiPackage(key),
			ImportPath: aws.ApiImportPath(key),
			Interface:  aws.ApiInterface(key),
			Methods:    aws.GeneratedApiMethods(key),
		})
	}
	return defs
}

func generateApiInterfaces() {
	templ, err := template.New("apis").Parse(apisTempl)
	if err != nil {
		panic(err)
	}

	writeTemplateToFile(templ, apiDefs(), FETCHERS_DIR, "gen_apis.go")
}

const apisTempl = `// Auto generated narrow interfaces over the AWS SDK

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

package awsfetch

// DO NOT EDIT - This file was automatically generated with go generate

// AWS SDK v2 ships no equivalent of v1's <service>iface packages, so awless-ro
// declares the operation sets it uses itself. Each service gets one interface
// composed of two halves: the operations the generated fetchers call, declared
// here, and the ones only the hand-written fetchers call, declared in
// manual_apis.go. Narrow interfaces are also what the SDK's own paginator
// constructors accept, so the generated fetchers can pass them straight through.
//
// The compile-time assertions at the bottom are what keep the hand-written half
// honest: if manual_apis.go names an operation the real client does not have, or
// gets a signature wrong, the build fails here rather than at the call site.

import (
	"context"

{{- range $, $api := . }}
	"{{ $api.ImportPath }}"
{{- end }}
)

{{ range $, $api := . }}
// {{ $api.Interface }} is every {{ $api.Package }} operation awless-ro calls.
type {{ $api.Interface }} interface {
	generated{{ $api.Interface }}
	manual{{ $api.Interface }}
}

type generated{{ $api.Interface }} interface {
{{- if not $api.Methods }}
	// no generated fetcher calls {{ $api.Package }} directly
{{- end }}
{{- range $, $method := $api.Methods }}
	{{ $method }}(context.Context, *{{ $api.Package }}.{{ $method }}Input, ...func(*{{ $api.Package }}.Options)) (*{{ $api.Package }}.{{ $method }}Output, error)
{{- end }}
}
{{ end }}

var (
{{- range $, $api := . }}
	_ {{ $api.Interface }} = (*{{ $api.Package }}.Client)(nil)
{{- end }}
)
`
