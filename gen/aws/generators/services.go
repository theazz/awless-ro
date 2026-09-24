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

// servicesView is what the services template renders; see fetchersView for why
// the imports are computed here.
type servicesView struct {
	Imports  []importLine
	Services interface{}
}

func generateServicesFuncs() {
	templ, err := template.New("services").Funcs(template.FuncMap{
		"Title":        aws.Title,
		"Join":         strings.Join,
		"ApiPackage":   aws.ApiPackage,
		"ApiField":     aws.ApiField,
		"ApiInterface": aws.ApiInterface,
	}).Parse(servicesTempl)

	if err != nil {
		panic(err)
	}

	var imports []importLine
	for _, api := range aws.UniqueApis() {
		imports = append(imports, importLine{Path: aws.ApiImportPath(api)})
	}
	for _, api := range aws.FetcherTypeApis() {
		imports = append(imports, importLine{Alias: aws.ApiTypesAlias(api), Path: aws.ApiImportPath(api) + "/types"})
	}

	view := servicesView{Imports: imports, Services: aws.FetchersDefs}
	writeTemplateToFile(templ, view, SERVICES_DIR, "gen_services.go")
}

const servicesTempl = `// Auto generated implementation for the AWS cloud service

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

import (
	"context"
	"errors"
	"sync"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
{{- range $, $imp := .Imports }}
	{{ $imp.Alias }} "{{ $imp.Path }}"
{{- end }}

	"github.com/theazz/awless-ro/aws/fetch"
	"github.com/theazz/awless-ro/cloud"
	"github.com/theazz/awless-ro/fetch"
	"github.com/theazz/awless-ro/graph"
	"github.com/theazz/awless-ro/logger"
	"github.com/theazz/awless-ro/triplestore"
)

var ServiceNames = []string{
{{- range $, $service := .Services }}
	"{{ $service.Name }}",
{{- end }}
}

var ResourceTypes = []string{
{{- range $, $service := .Services }}
{{- range $, $fetcher := $service.Fetchers }}
	"{{ $fetcher.ResourceType }}",
{{- end }}
{{- end }}
}

var ServicePerAPI = map[string]string{
{{- range $, $service := .Services }}
{{- range $, $api := $service.Api }}
	"{{ $api }}": "{{ $service.Name }}",
{{- end }}
{{- end }}
}

var ServicePerResourceType = map[string]string{
{{- range $, $service := .Services }}
{{- range $, $fetcher := $service.Fetchers }}
	"{{ $fetcher.ResourceType }}": "{{ $service.Name }}",
{{- end }}
{{- end }}
}

var APIPerResourceType = map[string]string{
{{- range $, $service := .Services }}
{{- range $, $fetcher := $service.Fetchers }}
	"{{ $fetcher.ResourceType }}": "{{ $fetcher.Api }}",
{{- end }}
{{- end }}
}

{{ range $, $service := .Services }}
type {{ Title $service.Name }} struct {
	fetcher         fetch.Fetcher
	region, profile string
	config          map[string]interface{}
	log             *logger.Logger
{{- range $, $api := $service.Api }}
	awsfetch.{{ ApiInterface $api }}
{{- end }}
}

func New{{ Title $service.Name }}(cfg awssdk.Config, profile string, extraConf map[string]interface{}, log *logger.Logger) cloud.Service {
{{- if $service.Global }}
	region := "global"
{{- else }}
	region := cfg.Region
{{- end }}

{{- range $, $api := $service.Api }}
	{{ $api }}API := {{ ApiPackage $api }}.NewFromConfig(cfg)
{{- end }}

	fetchConfig := awsfetch.NewConfig(&awsfetch.AWSAPI{
{{- range $, $api := $service.Api }}
		{{ ApiField $api }}: {{ $api }}API,
{{- end }}
	})
	fetchConfig.Extra = extraConf
	fetchConfig.Log = log

	return &{{ Title $service.Name }}{
{{- range $, $api := $service.Api }}
		{{ ApiInterface $api }}: {{ $api }}API,
{{- end }}
		fetcher: fetch.NewFetcher(awsfetch.Build{{ Title $service.Name }}FetchFuncs(fetchConfig)),
		config:  extraConf,
		region:  region,
		profile: profile,
		log:     log,
	}
}

func (s *{{ Title $service.Name }}) Name() string {
	return "{{ $service.Name }}"
}

func (s *{{ Title $service.Name }}) Region() string {
	return s.region
}

func (s *{{ Title $service.Name }}) Profile() string {
	return s.profile
}

func (s *{{ Title $service.Name }}) ResourceTypes() []string {
	return []string{
{{- range $, $fetcher := $service.Fetchers }}
		"{{ $fetcher.ResourceType }}",
{{- end }}
	}
}

func (s *{{ Title $service.Name }}) Fetch(ctx context.Context) (cloud.GraphAPI, error) {
	if s.IsSyncDisabled() {
		return graph.NewGraph(), nil
	}

	allErrors := new(fetch.Error)

	gph, err := s.fetcher.Fetch(context.WithValue(ctx, "region", s.region))
	defer s.fetcher.Reset()

	for _, e := range *fetch.WrapError(err) {
		switch {
		case e == nil:
			continue
		case awsfetch.IsAccessDenied(e):
			allErrors.Add(cloud.ErrFetchAccessDenied)
		default:
			allErrors.Add(e)
		}
	}

	if err := gph.AddResource(graph.InitResource(cloud.Region, s.region)); err != nil {
		return gph, err
	}

	snap := gph.AsRDFGraphSnaphot()

	errc := make(chan error)
	var wg sync.WaitGroup

{{- range $, $fetcher := $service.Fetchers }}
	if getBool(s.config, "aws.{{ $service.Name }}.{{ $fetcher.ResourceType }}.sync", true) {
		list, err := s.fetcher.Get("{{ $fetcher.ResourceType }}_objects")
		if err != nil {
			return gph, err
		}
		objects, ok := list.([]{{ $fetcher.AWSType }})
		if !ok {
			return gph, errors.New("cannot cast to '[]{{ $fetcher.AWSType }}' type from fetch context")
		}
		for _, r := range objects {
			for _, fn := range addParentsFns["{{ $fetcher.ResourceType }}"] {
				wg.Add(1)
				go func(f addParentFn, snap triplestore.RDFGraph, region string, res {{ $fetcher.AWSType }}) {
					defer wg.Done()
					if err := f(gph, snap, region, res); err != nil {
						errc <- err
					}
				}(fn, snap, s.region, r)
			}
		}
	}
{{- end }}

	go func() {
		wg.Wait()
		close(errc)
	}()

	for err := range errc {
		if err != nil {
			allErrors.Add(err)
		}
	}

	if allErrors.Any() {
		return gph, allErrors
	}

	return gph, nil
}

func (s *{{ Title $service.Name }}) FetchByType(ctx context.Context, t string) (cloud.GraphAPI, error) {
	defer s.fetcher.Reset()
	return s.fetcher.FetchByType(context.WithValue(ctx, "region", s.region), t)
}

func (s *{{ Title $service.Name }}) IsSyncDisabled() bool {
	return !getBool(s.config, "aws.{{ $service.Name }}.sync", true)
}
{{ end }}`
