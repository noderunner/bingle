package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/noderunner/bingle/internal/engine/dsl"
)

const dslUsage = `usage:
  bingle dsl convert FILE.json              print a JSON definition as YAML
  bingle dsl convert FILE.yaml              print a YAML definition as JSON, its includes
                                            resolved and every field name checked
  bingle dsl convert -                      print the JSON on standard input as YAML
`

// dslCommand runs "bingle dsl ...", which needs neither the configuration
// nor the database.
func dslCommand(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) != 2 || args[0] != "convert" {
		_, _ = fmt.Fprint(errOut, dslUsage)
		return 2
	}
	data, err := convertDefinition(args[1], in)
	if err != nil {
		_, _ = fmt.Fprintln(errOut, "bingle:", err)
		return 1
	}
	_, _ = out.Write(data)
	return 0
}

// convertDefinition converts the definition at name (or on in, for "-") to
// the other format.
func convertDefinition(name string, in io.Reader) ([]byte, error) {
	switch ext := strings.ToLower(filepath.Ext(name)); {
	case name == "-":
		data, err := io.ReadAll(in)
		if err != nil {
			return nil, err
		}
		return dsl.ToYAML(data)
	case ext == ".json":
		data, err := os.ReadFile(name)
		if err != nil {
			return nil, err
		}
		return dsl.ToYAML(data)
	case ext == ".yaml" || ext == ".yml":
		// The definition's folder is its root: an include cannot reach
		// outside it, through ".." or a symbolic link.
		root, err := os.OpenRoot(filepath.Dir(name))
		if err != nil {
			return nil, err
		}
		defer func() { _ = root.Close() }()
		data, err := dsl.Load(root.FS(), filepath.Base(name))
		if err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		if err := json.Indent(&buf, data, "", "  "); err != nil {
			return nil, err
		}
		buf.WriteByte('\n')
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("%s: a definition is a .json, .yaml or .yml file", name)
}
