package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/Raoon-Tech/bpmn-ai/api/grpc"
	"github.com/Raoon-Tech/bpmn-ai/api/http"
	"github.com/Raoon-Tech/bpmn-ai/api/middleware"
	"github.com/Raoon-Tech/bpmn-ai/config"
	"github.com/Raoon-Tech/bpmn-ai/internal/bootstrap"
	"github.com/Raoon-Tech/bpmn-ai/internal/observability"
	"github.com/Raoon-Tech/bpmn-ai/pkg/store/migrate"
)

func cmdServe(args []string) int {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprintln(os.Stderr, "usage: bpmn-ai serve") }
	if err := fs.Parse(reorderArgs(args)); err != nil {
		return exitUsage
	}
	if fs.NArg() > 0 {
		return usageError("serve", fmt.Sprintf("unexpected argument %q", fs.Arg(0)))
	}

	cfg, err := loadConfig()
	if err != nil {
		return fail("invalid configuration: %v", err)
	}

	logger, err := observability.NewFromConfig(cfg.Log.Level, cfg.Log.Format)
	if err != nil {
		return fail("failed to create logger: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	target := bootstrap.Describe(cfg.Database)
	rt, err := bootstrap.NewRuntime(ctx, cfg, logger)
	if err != nil {
		logger.Error("cannot open database", "driver", cfg.Database.Driver, "target", target, "error", err)
		return exitError
	}
	defer func() { _ = rt.Close() }()
	logger.Info("store ready", "target", target)

	if rt.DB != nil {
		if err := ensureMigrations(ctx, cfg, rt, logger); err != nil {
			logger.Error("database migrations failed", "error", err)
			return exitError
		}
	}

	rt.Start(ctx)

	srv := http.NewServer(http.ServerConfig{
		Host:         cfg.Server.Host,
		Port:         cfg.Server.Port,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  60 * time.Second,
		DisableCSRF:  cfg.Server.DisableCSRF,
		Auth: middleware.AuthConfig{
			Mode:             cfg.Auth.Mode,
			JWKSURL:          cfg.Auth.JWKSURL,
			Issuer:           cfg.Auth.Issuer,
			Audience:         cfg.Auth.Audience,
			UserClaim:        cfg.Auth.UserClaim,
			RolesClaim:       cfg.Auth.RolesClaim,
			TenantClaim:      cfg.Auth.TenantClaim,
			TrustedProxies:   cfg.Auth.TrustedProxies,
			EnforceTaskRoles: cfg.Auth.EnforceTaskRoles,
		},
		TLSCertFile:  cfg.Server.TLSCertFile,
		TLSKeyFile:   cfg.Server.TLSKeyFile,
	}, rt.Store, rt.Engine, rt.Queue, logger, observability.DefaultMetrics())

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		if err := srv.Start(); err != nil {
			logger.Error("server error", "error", err)
		}
	}()

	if cfg.Server.GrpcEnabled {
		grpcSvc := &grpc.Service{
			Store:  rt.Store,
			Engine: rt.Engine,
			Logger: logger,
		}
		grpcSrv := grpc.NewServer(fmt.Sprintf(":%d", cfg.Server.GrpcPort), grpcSvc, logger)
		go func() {
			logger.Info("grpc server starting", "addr", fmt.Sprintf(":%d", cfg.Server.GrpcPort))
			if err := grpcSrv.Start(); err != nil {
				logger.Error("grpc server error", "error", err)
			}
		}()
		defer grpcSrv.Stop()
	}

	<-stop
	logger.Info("shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	rt.Stop()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", "error", err)
		return exitError
	}

	logger.Info("server stopped")
	return exitOK
}

func ensureMigrations(ctx context.Context, cfg config.Config, rt *bootstrap.Runtime, logger *observability.Logger) error {
	if rt.DB == nil {
		return nil
	}
	if cfg.Database.AutoMigrate {
		applied, err := migrate.Up(ctx, rt.DB, migrate.Options{})
		if err != nil {
			return err
		}
		if len(applied) > 0 {
			logger.Info("migrations applied", "versions", applied)
		}
		return nil
	}

	pending, err := migrate.Pending(ctx, rt.DB, migrate.Options{})
	if err != nil {
		return err
	}
	if len(pending) > 0 {
		return fmt.Errorf("%d pending migration(s): %s (run \"bpmn-ai migrate up\" or set DB_AUTO_MIGRATE=true)",
			len(pending), strings.Join(pending, ", "))
	}
	return nil
}
