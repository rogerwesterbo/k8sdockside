package kube

// A merge patch typed as text, on its way to several objects at once.
//
// The form that builds one offers a YAML editor beside its fields, so a patch
// touching several places -- two labels and a replica count -- is written the
// way the objects themselves are read. The YAML is read here rather than in
// the frontend for the reason the object editor's check is: there is one
// parser, and it is the one that will actually be asked to read the patch.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	sigsyaml "sigs.k8s.io/yaml"
)

// PatchPreview is the answer to the patch form's question while a merge patch
// is typed: what it comes to as JSON, or what is wrong with it.
type PatchPreview struct {
	// JSON is the compact merge patch each object would receive.
	JSON string `json:"json"`
	// Empty reports a document holding nothing -- blank, or comments only --
	// which is not a mistake worth marking, only not a patch yet.
	Empty bool `json:"empty"`
	// Error and Line are the parser's complaint, Line 1-based and 0 when it
	// named none, as YAMLCheck carries them for the editor.
	Error string `json:"error"`
	Line  int    `json:"line"`
}

// MergePatchFromYAML reads a merge patch written as YAML -- or as JSON, which
// is YAML too -- into the compact JSON the API server takes.
//
// It refuses what is not a mapping: a list or a bare scalar is not a patch of
// anything, and yaml.v3 says so with a line number where a looser decode would
// hand back something to send.
func MergePatchFromYAML(text string) PatchPreview {
	if strings.TrimSpace(text) == "" {
		return PatchPreview{Empty: true}
	}

	var body map[string]any
	if err := yaml.Unmarshal([]byte(text), &body); err != nil {
		message, line := explainYAML(err)
		return PatchPreview{Error: message, Line: line}
	}
	if len(body) == 0 {
		return PatchPreview{Empty: true}
	}

	// Through sigs.k8s.io/yaml rather than json.Marshal of the map above: it
	// is the conversion kubectl makes, and it settles YAML's own types -- a
	// bare number, a quoted one -- the way the API server will read them.
	raw, err := sigsyaml.YAMLToJSON([]byte(text))
	if err != nil {
		message, line := explainYAML(err)
		return PatchPreview{Error: message, Line: line}
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw); err != nil {
		return PatchPreview{Error: err.Error()}
	}
	return PatchPreview{JSON: compact.String()}
}

// mergePatchBytes is MergePatchFromYAML for the caller that needs the bytes or
// a reason to stop: nothing is sent to any object over a document that is not
// a patch.
func mergePatchBytes(text string) ([]byte, error) {
	preview := MergePatchFromYAML(text)
	switch {
	case preview.Error != "" && preview.Line > 0:
		return nil, fmt.Errorf("the patch is not valid: line %d: %s", preview.Line, preview.Error)
	case preview.Error != "":
		return nil, fmt.Errorf("the patch is not valid: %s", preview.Error)
	case preview.Empty:
		return nil, errors.New("the patch is empty")
	}
	return []byte(preview.JSON), nil
}
