// Package assets holds the go:embed defaults baked into the binary so
// `cage init` works anywhere with zero external files.
package assets

import _ "embed"

//go:embed config.default.yaml
var ConfigDefault []byte

//go:embed dod.example.yaml
var DODExample []byte

//go:embed brain.template.md
var BrainTemplate []byte
