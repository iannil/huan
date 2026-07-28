// Package translate re-exports the Translator capability contract from
// github.com/iannil/huan/pkg/translate so huan-internal call sites keep
// importing "internal/translate" while sharing the SAME types with out-of-tree
// .so plugins.
//
// The canonical definitions live in pkg/translate (public) because Go interface
// satisfaction requires identical named types across the .so boundary — an
// out-of-tree plugin module cannot import internal/ packages, so the contract
// must be public. These aliases keep existing references (translate.Request,
// translate.Response, translate.Translator, ...) working unchanged, including
// the QualityResult.HardCheckFailures method (carried by the alias).
//
// See docs/adr/0008-translator-capability-qwen3-plugin.md and pkg/translate/types.go.
package translate

import pkgtranslate "github.com/iannil/huan/pkg/translate"

// Capability contract + shared types, aliased from pkg/translate so .so plugins
// and huan internal code share the same type identity (mirrors how
// internal/plugin aliases pkg/plugin and internal/deploy aliases pkg/deploy).
type (
	Translator    = pkgtranslate.Translator
	Request       = pkgtranslate.Request
	Response      = pkgtranslate.Response
	QualityResult = pkgtranslate.QualityResult
)
