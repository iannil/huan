package plugin

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// capabilityContractWhitelist lists capability interfaces intentionally allowed
// OUTSIDE pkg/, each with the reason. Keep this list shrinking.
var capabilityContractWhitelist = map[string]string{
	// EventSubscriber references internal/daemon/eventbus types and has no .so
	// implementer today; migrating it (and the eventbus types it needs) to pkg/
	// is deferred (YAGNI) until the first .so plugin needs event subscription.
	"EventSubscriber": "references internal/daemon/eventbus; no .so implementer; deferred (YAGNI)",
	// GRPCPlugin is served over gRPC (a cross-process transport), so interface
	// satisfaction never crosses a Go .so boundary — the type-identity hazard
	// does not apply. Reserved/unimplemented.
	"GRPCPlugin": "gRPC cross-process transport; not subject to the in-process .so type-identity hazard",
	// Hook (internal/build) is the strongly-typed, in-process build-pipeline
	// contract: its signatures use *content.Page, which pulls in internal/content
	// and therefore cannot live under pkg/. The .so-safe counterpart already
	// exists as pkg/plugin.Hook (same methods, interface{} page refs); .so
	// plugins import that one. This internal alias is host-only convenience, so
	// it is not subject to the cross-.so type-identity hazard.
	"Hook": "internal build-pipeline contract using *content.Page (not pkg-importable); .so-safe counterpart is pkg/plugin.Hook",
}

// TestCapabilityContractsLiveInPkg asserts every capability interface — one
// that embeds plugin.Plugin and adds at least one method — is declared under
// pkg/, so .so plugins can import the exact host type. A contract defined in
// internal/ causes silent cross-.so interface mismatch (see the deploy /
// translate bugs of 2026-07-28). Intentional exceptions go in
// capabilityContractWhitelist with a reason.
func TestCapabilityContractsLiveInPkg(t *testing.T) {
	roots := []string{"../../internal", "../../pkg", "../../cmd"}
	fset := token.NewFileSet()
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, perr := parser.ParseFile(fset, path, nil, 0)
			if perr != nil {
				return perr
			}
			ast.Inspect(f, func(n ast.Node) bool {
				ts, ok := n.(*ast.TypeSpec)
				if !ok {
					return true
				}
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					return true // type aliases / non-interface specs are skipped
				}
				if !embedsPlugin(it) || !addsMethod(it) {
					return true
				}
				if strings.Contains(filepath.ToSlash(path), "/pkg/") {
					return true // correctly placed
				}
				if _, ok := capabilityContractWhitelist[ts.Name.Name]; ok {
					return true // known, documented exception
				}
				t.Errorf("capability interface %q in %s must live under pkg/ "+
					"(it embeds plugin.Plugin and adds methods; .so plugins cannot "+
					"import internal/). Move it to pkg/, or add it to "+
					"capabilityContractWhitelist with a reason.", ts.Name.Name, path)
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}

// embedsPlugin reports whether the interface embeds a type named Plugin
// (either bare `Plugin` or a selector like `pkgplugin.Plugin`).
func embedsPlugin(it *ast.InterfaceType) bool {
	for _, field := range it.Methods.List {
		if len(field.Names) != 0 {
			continue // a method, not an embed
		}
		switch e := field.Type.(type) {
		case *ast.Ident:
			if e.Name == "Plugin" {
				return true
			}
		case *ast.SelectorExpr:
			if e.Sel != nil && e.Sel.Name == "Plugin" {
				return true
			}
		}
	}
	return false
}

// addsMethod reports whether the interface declares at least one method
// (beyond embedded interfaces).
func addsMethod(it *ast.InterfaceType) bool {
	for _, field := range it.Methods.List {
		if len(field.Names) != 0 {
			return true
		}
	}
	return false
}
