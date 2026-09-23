//go:generate go run $GOFILE properties.go apis.go fetchers.go services.go mocks.go

// The generator gofmts every file it writes, so there is no post-processing
// pass here. Upstream also ran `goimports -w` over the output directories; that
// is gone, because goimports is not a Go distribution tool and requiring it made
// `make generate` fail on a clean checkout. The templates spell out their own
// import blocks instead.

// Command generators is the single entry point for all code generation in
// awless-ro. It reads the declarations in gen/aws/*_definitions.go and emits
// the gen_*.go files into aws/fetch, aws/services, cloud/properties and
// cloud/rdf. Never edit a generated file by hand: change the declarations and
// run `make generate` instead.
package main

import (
	"bytes"
	"flag"
	"go/format"
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

	// AWS API interfaces, then the fetchers, services and mocks built on them
	generateApiInterfaces()
	generateFetcherFuncs()
	generateServicesFuncs()
	generateTestMocks()

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

	// Templates are written for readability of the template, not of its output,
	// so the result is gofmt-ed here. When it does not parse, the unformatted
	// output is still written out: reading the broken file is the only way to see
	// what the template actually produced.
	src, formatErr := format.Source(buff.Bytes())
	if formatErr != nil {
		src = buff.Bytes()
	}
	if err := os.WriteFile(path, src, 0666); err != nil {
		log.Fatal(err)
	}
	if formatErr != nil {
		log.Fatalf("%s does not parse, left unformatted: %s", relativePathToRoot(path), formatErr)
	}

	log.Printf("generated %s", relativePathToRoot(path))
}

func relativePathToRoot(path string) string {
	rel, _ := filepath.Rel(ROOT_DIR, path)
	return rel
}
