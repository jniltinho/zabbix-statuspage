package server

import (
	stdctx "context"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"syscall"
	"time"

	"zabbix-statuspage/internal/config"
	"zabbix-statuspage/internal/handler"
	"zabbix-statuspage/internal/zabbix"
	"github.com/labstack/echo/v5"
)

type Server struct {
	cfg     *config.Config
	webFS   fs.FS
	debug   bool
	version string
}

func New(cfg *config.Config, webFS fs.FS, debug bool, version string) *Server {
	return &Server{cfg: cfg, webFS: webFS, debug: debug, version: version}
}

func (s *Server) Start() error {
	templatesFS, err := fs.Sub(s.webFS, "web/templates")
	if err != nil {
		return fmt.Errorf("templates fs: %w", err)
	}
	staticFS, err := fs.Sub(s.webFS, "web/static")
	if err != nil {
		return fmt.Errorf("static fs: %w", err)
	}

	renderer, err := newRenderer(templatesFS)
	if err != nil {
		return err
	}

	zabbixClient := zabbix.New(s.cfg.Zabbix.APIURL, s.cfg.Zabbix.APIToken, s.cfg.Zabbix.CacheTTL)
	statusHandler := handler.New(zabbixClient, s.cfg, s.debug, s.version)

	e := echo.New()
	e.Renderer = renderer
	registerRoutes(e, statusHandler, staticFS)

	addr := fmt.Sprintf("%s:%d", s.cfg.Server.Addr, s.cfg.Server.Port)

	ctx, stop := signal.NotifyContext(stdctx.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sc := echo.StartConfig{
		Address:         addr,
		HideBanner:      true,
		GracefulTimeout: 10 * time.Second,
	}

	if s.cfg.Server.TLS {
		fmt.Printf("https server starting on %s\n", addr)
		return sc.StartTLS(ctx, e, s.cfg.Server.TLSCert, s.cfg.Server.TLSKey)
	}
	fmt.Printf("http server starting on %s\n", addr)
	return sc.Start(ctx, e)
}
