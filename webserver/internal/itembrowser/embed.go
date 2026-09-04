package itembrowser

import _ "embed"

// indexHTML is the whole UI: one self-contained page with no external assets,
// so the tool works offline and needs no build step.
//
//go:embed index.html
var indexHTML []byte
