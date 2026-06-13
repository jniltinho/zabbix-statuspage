package server

import (
	"io/fs"

	"zabbix-statuspage/internal/handler"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func registerRoutes(e *echo.Echo, statusHandler *handler.StatusHandler, staticFS fs.FS) {
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.Gzip())

	e.GET("/", statusHandler.Handle)
	e.GET("/static/*", echo.StaticDirectoryHandler(staticFS, false))
}
