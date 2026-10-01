package docs

import "embed"

// Files contains the documentation shipped with this exact Glance build.
//
// The existing Markdown and image files remain the source of truth for both
// GitHub and the in-application documentation viewer.
//
//go:embed *
var Files embed.FS
