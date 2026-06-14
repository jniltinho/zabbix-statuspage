package server

import (
	"crypto/subtle"
	"io/fs"

	"zabbix-statuspage/internal/config"
	"zabbix-statuspage/internal/handler"
	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

func registerRoutes(e *echo.Echo, statusHandler *handler.StatusHandler, staticFS fs.FS, cfg *config.Config) {
	e.Use(middleware.RequestLogger())
	e.Use(middleware.Recover())
	e.Use(middleware.Gzip())

	if cfg.Server.BasicAuth.Enabled {
		user := []byte(cfg.Server.BasicAuth.Username)
		pass := []byte(cfg.Server.BasicAuth.Password)
		e.Use(middleware.BasicAuth(func(_ *echo.Context, u, p string) (bool, error) {
			okUser := subtle.ConstantTimeCompare([]byte(u), user) == 1
			okPass := subtle.ConstantTimeCompare([]byte(p), pass) == 1
			return okUser && okPass, nil
		}))
	}

	e.GET("/", statusHandler.Handle)
	e.GET("/static/*", echo.StaticDirectoryHandler(staticFS, false))
}
