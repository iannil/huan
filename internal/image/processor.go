// Package image re-exports the ImageProcessor capability contract from
// pkg/image via a type alias, so existing internal call sites keep importing
// internal/image while the real type lives in pkg/ (shared with .so plugins).
package image

import pkgimage "github.com/iannil/huan/pkg/image"

// ImageProcessor is an alias of pkg/image.ImageProcessor. Both names denote the
// exact same type, so .so plugins importing pkg/image and internal code
// importing internal/image satisfy one identical interface across the .so
// boundary.
type ImageProcessor = pkgimage.ImageProcessor
