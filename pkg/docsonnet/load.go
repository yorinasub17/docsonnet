package docsonnet

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"github.com/google/go-jsonnet"
)

type Opts struct {
	JPath      []string
	EmbeddedFS embed.FS
}

// RenderWithJsonnet uses the jsonnet render function to generate the docs, instead of the golang utilities.
func RenderWithJsonnet(filename string, opts Opts) (map[string]string, error) {
	// get render.libsonnet from embedded data
	render, err := opts.EmbeddedFS.ReadFile("render.libsonnet")
	if err != nil {
		return nil, err
	}

	// setup Jsonnet vm
	vm, err := newVM(filename, opts)
	if err != nil {
		return nil, err
	}

	// invoke render.libsonnet
	vm.ExtCode("d", `(import "doc-util/main.libsonnet")`)

	data, err := vm.EvaluateAnonymousSnippet("render.libsonnet", string(render))
	if err != nil {
		return nil, err
	}

	var out map[string]string
	err = json.Unmarshal([]byte(data), &out)
	return out, err
}

// Load extracts and transforms the docsonnet data in `filename`, returning the
// top level docsonnet package.
func Load(filename string, opts Opts) (*Package, error) {
	data, err := Extract(filename, opts)
	if err != nil {
		return nil, err
	}

	return Transform([]byte(data))
}

// Extract parses the Jsonnet file at `filename`, extracting all docsonnet related
// information, exactly as they appear in Jsonnet. Keep in mind this
// representation is usually not suitable for any use, use `Transform` to
// convert it to the familiar docsonnet data model.
func Extract(filename string, opts Opts) ([]byte, error) {
	// get load.libsonnet from embedded data
	load, err := opts.EmbeddedFS.ReadFile("load.libsonnet")
	if err != nil {
		return nil, err
	}

	// setup Jsonnet vm
	vm, err := newVM(filename, opts)
	if err != nil {
		return nil, err
	}

	// invoke load.libsonnet
	data, err := vm.EvaluateAnonymousSnippet("load.libsonnet", string(load))
	if err != nil {
		return nil, err
	}

	return []byte(data), nil
}

// Transform converts the raw result of `Extract` to the actual docsonnet object
// model `*docsonnet.Package`.
func Transform(data []byte) (*Package, error) {
	var d ds
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		log.Fatalln(err)
	}

	p := fastLoad(d)
	return &p, nil
}

// newVM sets up the Jsonnet VM with the importer that statically provides doc-util.
func newVM(mainFName string, opts Opts) (*jsonnet.VM, error) {
	vm := jsonnet.MakeVM()
	imp, err := newImporter(opts)
	if err != nil {
		return nil, err
	}
	vm.Importer(imp)
	vm.ExtCode("main", fmt.Sprintf(`(import "%s")`, mainFName))
	return vm, nil
}

// importer wraps jsonnet.FileImporter, to statically provide doc-util,
// bundled with the binary
type importer struct {
	fi       jsonnet.FileImporter
	embedded map[string]jsonnet.Contents
}

func newImporter(opts Opts) (*importer, error) {
	dmain, err := opts.EmbeddedFS.ReadFile("doc-util/main.libsonnet")
	if err != nil {
		return nil, err
	}
	drender, err := opts.EmbeddedFS.ReadFile("doc-util/render.libsonnet")
	if err != nil {
		return nil, err
	}
	embedded := map[string]jsonnet.Contents{
		"main.libsonnet":   jsonnet.MakeContents(string(dmain)),
		"render.libsonnet": jsonnet.MakeContents(string(drender)),
	}

	return &importer{
		fi:       jsonnet.FileImporter{JPaths: opts.JPath},
		embedded: embedded,
	}, nil
}

var docUtilPathPrefixes = []string{
	"doc-util/",
	"github.com/jsonnet-libs/docsonnet/doc-util/",
	"./render.libsonnet",
}

func (i *importer) Import(importedFrom, importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	for _, p := range docUtilPathPrefixes {
		if strings.HasPrefix(importedPath, p) {
			return i.loadFromEmbed(importedPath)
		}
	}

	return i.fi.Import(importedFrom, importedPath)
}

func (i *importer) loadFromEmbed(importedPath string) (contents jsonnet.Contents, foundAt string, err error) {
	fbase := filepath.Base(importedPath)
	fpath := filepath.Join("doc-util", fbase)
	loadPath := fmt.Sprintf("<internal>/%s", fpath)

	conts, hasConts := i.embedded[fbase]
	if !hasConts {
		return jsonnet.Contents{}, loadPath, fmt.Errorf("%s does not exist", fpath)
	}
	return conts, loadPath, nil
}
