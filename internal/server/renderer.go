package server

import (
	"fmt"
	"html/template"
	"io/fs"

	"github.com/labstack/echo/v5"
)

func newRenderer(templatesFS fs.FS) (*echo.TemplateRenderer, error) {
	tmpl, err := template.ParseFS(templatesFS, "*.html")
	if err != nil {
		return nil, fmt.Errorf("parse templates: %w", err)
	}
	return &echo.TemplateRenderer{Template: tmpl}, nil
}
