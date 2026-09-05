package npcpanel

import _ "embed"

// indexHTML is the panel UI: one self-contained page, no build step and no
// external requests, matching the itembrowser tool it sits beside.
//
//go:embed index.html
var indexHTML []byte
