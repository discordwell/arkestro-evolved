package web

import "embed"

// TemplatesFS contains HTML templates for the server-rendered UI.
//
//go:embed templates/*.html
var TemplatesFS embed.FS

// StaticFS contains the CSS and other static assets.
//
//go:embed static/*
var StaticFS embed.FS
