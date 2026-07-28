// Package deploy re-exports the Deployer capability contract from
// github.com/iannil/huan/pkg/deploy so huan-internal call sites keep importing
// "internal/deploy" while sharing the SAME types with out-of-tree .so plugins.
//
// The canonical definitions live in pkg/deploy (public) because Go interface
// satisfaction requires identical named types across the .so boundary — an
// out-of-tree plugin module cannot import internal/ packages, so the contract
// must be public. These aliases keep existing references (deploy.Options,
// deploy.Report, deploy.Deployer, ...) working unchanged.
//
// See docs/adr/0003-unified-plugin-system.md and pkg/deploy/types.go.
package deploy

import pkgdeploy "github.com/iannil/huan/pkg/deploy"

// Capability contract + shared types, aliased from pkg/deploy so .so plugins
// and huan internal code share the same type identity (mirrors how
// internal/plugin aliases pkg/plugin).
type (
	Deployer     = pkgdeploy.Deployer
	Options      = pkgdeploy.Options
	PagesOptions = pkgdeploy.PagesOptions
	R2Options    = pkgdeploy.R2Options
	Report       = pkgdeploy.Report
	FileFailure  = pkgdeploy.FileFailure
)
