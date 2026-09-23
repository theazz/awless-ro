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
	"strings"
	"text/template"

	"github.com/theazz/awless-ro/gen/aws"
)

// guardPath returns the expression that has to be non-nil before an outputs
// extractor can be dereferenced, or "" when the extractor reads a field of the
// output directly. SDK v2 keeps nested shapes behind pointers, so an extractor
// such as "DistributionList.Items" panics on an empty response without this.
func guardPath(extractor string) string {
	idx := strings.LastIndex(extractor, ".")
	if idx < 0 {
		return ""
	}
	return "out." + extractor[:idx]
}

// importLine is one entry of a generated import block.
type importLine struct {
	Alias string
	Path  string
}

// fetchersView is what the fetchers template renders. Imports are computed here
// rather than derived in the template so that services reached only through
// hand-written fetchers, or not fetched at all, do not end up imported and
// unused.
type fetchersView struct {
	Imports  []importLine
	Services interface{}
}

func generateFetcherFuncs() {
	templ, err := template.New("funcs").Funcs(template.FuncMap{
		"Title":      aws.Title,
		"Join":       strings.Join,
		"ApiPackage": aws.ApiPackage,
		"ApiField":   aws.ApiField,
		"GuardPath":  guardPath,
	}).Parse(fetchersTempl)

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

	view := fetchersView{Imports: imports, Services: aws.FetchersDefs}
	writeTemplateToFile(templ, view, FETCHERS_DIR, "gen_fetchers.go")
}

const fetchersTempl = `// Auto generated implementation for the AWS cloud service

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

import (
	"context"

{{- range $, $imp := .Imports }}
	{{ $imp.Alias }} "{{ $imp.Path }}"
{{- end }}

	"github.com/theazz/awless-ro/aws/conv"
	"github.com/theazz/awless-ro/fetch"
	"github.com/theazz/awless-ro/graph"
)

{{- range $, $service := .Services }}
func Build{{ Title $service.Name }}FetchFuncs(conf *Config) fetch.Funcs {
	funcs := make(map[string]fetch.Func)

	addManual{{ Title $service.Name }}FetchFuncs(conf, funcs)

{{- range $, $fetcher := $service.Fetchers }}
{{- if not $fetcher.ManualFetcher }}

	funcs["{{ $fetcher.ResourceType }}"] = func(ctx context.Context, cache fetch.Cache) ([]*graph.Resource, interface{}, error) {
		var resources []*graph.Resource
		var objects []{{ $fetcher.AWSType }}

		if !conf.getBoolDefaultTrue("aws.{{ $service.Name }}.{{ $fetcher.ResourceType }}.sync") && !getBoolFromContext(ctx, "force") {
			conf.Log.Verbose("sync: *disabled* for resource {{ $service.Name }}[{{ $fetcher.ResourceType }}]")
			return resources, objects, nil
		}

		input := &{{ ApiPackage $fetcher.Api }}.{{ $fetcher.ApiMethod }}Input{ {{ $fetcher.InputFields }} }
{{- if $fetcher.Paginated }}

		paginator := {{ ApiPackage $fetcher.Api }}.New{{ $fetcher.ApiMethod }}Paginator(conf.APIs.{{ ApiField $fetcher.Api }}, input)
		for paginator.HasMorePages() {
			out, err := paginator.NextPage(ctx)
			if err != nil {
				return resources, objects, err
			}
{{ template "collect" $fetcher }}
		}

		return resources, objects, nil
{{- else }}

		out, err := conf.APIs.{{ ApiField $fetcher.Api }}.{{ $fetcher.ApiMethod }}(ctx, input)
		if err != nil {
			return resources, objects, err
		}
{{ template "collect" $fetcher }}

		return resources, objects, nil
{{- end }}
	}
{{- end }}
{{- end }}
	return funcs
}
{{ end }}

{{- define "collect" }}
{{- if .OutputsContainers }}
			for _, container := range out.{{ .OutputsContainers }} {
				for _, output := range container.{{ .OutputsExtractor }} {
					objects = append(objects, output)
					res, err := awsconv.NewResource(output)
					if err != nil {
						return resources, objects, err
					}
					resources = append(resources, res)
				}
			}
{{- else }}
{{- if GuardPath .OutputsExtractor }}
			if {{ GuardPath .OutputsExtractor }} != nil {
{{- end }}
			for _, output := range out.{{ .OutputsExtractor }} {
				objects = append(objects, output)
				res, err := awsconv.NewResource(output)
				if err != nil {
					return resources, objects, err
				}
				resources = append(resources, res)
			}
{{- if GuardPath .OutputsExtractor }}
			}
{{- end }}
{{- end }}
{{- end }}
`
