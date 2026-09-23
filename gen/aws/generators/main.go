//go:generate go run $GOFILE properties.go fetchers.go services.go

//go:generate gofmt -s -w ../../../aws
//go:generate goimports -w ../../../aws

//go:generate gofmt -s -w ../../../aws/services
//go:generate goimports -w ../../../aws/services

//go:generate gofmt -s -w ../../../aws/fetch
//go:generate goimports -w ../../../aws/fetch

//go:generate gofmt -s -w ../../../cloud/properties
//go:generate goimports -w ../../../cloud/properties

//go:generate gofmt -s -w ../../../cloud/rdf
//go:generate goimports -w ../../../cloud/rdf

// Command generators is the single entry point for all code generation in
// awless-ro. It reads the declarations in gen/aws/*_definitions.go and emits
// the gen_*.go files into aws/fetch, aws/services, cloud/properties and
// cloud/rdf. Never edit a generated file by hand: change the declarations and
// run `make generate` instead.
package main

import (
	"bytes"
	"flag"
	"log"
	"os"
	"path/filepath"

	"text/template"
)

var (
	ROOT_DIR = filepath.Join("..", "..", "..")

	FETCHERS_DIR         = filepath.Join(ROOT_DIR, "aws", "fetch")
	SERVICES_DIR         = filepath.Join(ROOT_DIR, "aws", "services")
	CLOUD_PROPERTIES_DIR = filepath.Join(ROOT_DIR, "cloud", "properties")
	CLOUD_RDF_DIR        = filepath.Join(ROOT_DIR, "cloud", "rdf")
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("[+] ")
	flag.Parse()

	// fetchers
	generateFetcherFuncs()
	generateServicesFuncs()

	// properties
	generateProperties()
	generateRDFProperties()
}

func writeTemplateToFile(templ *template.Template, data interface{}, dir, filename string) {
	var buff bytes.Buffer
	if err := templ.Execute(&buff, data); err != nil {
		log.Fatal(err)
	}
	path := filepath.Join(dir, filename)
	if err := os.WriteFile(path, buff.Bytes(), 0666); err != nil {
		log.Fatal(err)
	}

	log.Printf("generated %s", relativePathToRoot(path))
}

func relativePathToRoot(path string) string {
	rel, _ := filepath.Rel(ROOT_DIR, path)
	return rel
}
