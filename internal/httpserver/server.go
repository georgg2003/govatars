package httpserver

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	oapiMiddleware "github.com/oapi-codegen/echo-middleware"

	"govatars/api"
	httphandler "govatars/internal/delivery/http"
	"govatars/internal/delivery/web"
	"govatars/internal/pkg/contextlib"
	srvmw "govatars/internal/pkg/middleware"
	"govatars/internal/serverapp"

	"go.opentelemetry.io/contrib/instrumentation/github.com/labstack/echo/otelecho"
)

// Run builds Echo, serves until ctx is cancelled, then shuts down gracefully.
func Run(ctx context.Context, application *serverapp.App) error {
	logger := application.Logger
	cfg := application.Cfg

	e := echo.New()
	e.HideBanner = true

	if application.OTELTracerProvider != nil {
		e.Use(
			otelecho.Middleware(
				"govatars",
				otelecho.WithTracerProvider(application.OTELTracerProvider.TracerProvider),
			),
		)
	}
	if application.OTELMetricsProvider != nil {
		e.Use(
			otelecho.Middleware(
				"govatars",
				otelecho.WithMeterProvider(application.OTELMetricsProvider.MeterProvider),
			),
		)
	}
	e.Use(middleware.RequestID())
	e.Use(srvmw.RequestUserID())
	e.Use(srvmw.RequestContext())
	e.Use(srvmw.AccessLog(logger))
	e.Use(middleware.RecoverWithConfig(middleware.RecoverConfig{
		DisablePrintStack: true,
		//nolint:contextcheck // Echo RecoverConfig does not pass context into LogErrorFunc.
		LogErrorFunc: func(c echo.Context, err error, stack []byte) error {
			logger.ErrorContext(c.Request().Context(), "panic_recovered",
				"err", err.Error(),
				"stack", string(stack),
			)
			return err
		},
	}))
	e.Use(middleware.BodyLimit(EchoBodyLimit(cfg)))

	if len(cfg.HTTP.CORSAllowOrigins) > 0 {
		e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
			AllowOrigins: cfg.HTTP.CORSAllowOrigins,
			AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions, http.MethodHead},
			AllowHeaders: []string{echo.HeaderOrigin, echo.HeaderContentType, echo.HeaderAccept, contextlib.HeaderXUserID},
		}))
	}

	if cfg.HTTP.RateLimit.RequestsPerSecond > 0 {
		burst := cfg.HTTP.RateLimit.Burst
		if burst <= 0 {
			burst = int(cfg.HTTP.RateLimit.RequestsPerSecond * 2)
		}
		store := middleware.NewRateLimiterMemoryStoreWithConfig(middleware.RateLimiterMemoryStoreConfig{
			Rate:      rate.Limit(cfg.HTTP.RateLimit.RequestsPerSecond),
			Burst:     burst,
			ExpiresIn: 3 * time.Minute,
		})
		e.Use(middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{Store: store}))
	}

	swagger, err := openapi3.NewLoader().LoadFromData(api.SwaggerYAML)
	if err != nil {
		return err
	}
	oapiMW := oapiMiddleware.OapiRequestValidatorWithOptions(swagger, &oapiMiddleware.Options{})
	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p := c.Request().URL.Path
			if strings.HasPrefix(p, "/web") {
				return next(c)
			}
			if c.Request().Method == http.MethodPost && p == "/api/v1/avatars" {
				return next(c)
			}
			return oapiMW(next)(c)
		}
	})

	thumbs, err := cfg.Avatars.Catalog()
	if err != nil {
		return fmt.Errorf("avatars catalog: %w", err)
	}
	srv := httphandler.NewServer(
		application.Health,
		application.Avatar,
		cfg.Avatars.MaxUploadBytes,
		thumbs.Labels,
		httphandler.WithLogger(logger),
	)
	//nolint:contextcheck // Route registration; each request still carries context via Echo.
	web.New(application.Avatar, cfg.HTTP.StaticDir, web.WithLogger(logger)).Register(e)
	httphandler.RegisterHandlers(e, srv)

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		defer cancel()
		logger.InfoContext(shutdownCtx, "shutting down http server")
		if err := e.Shutdown(shutdownCtx); err != nil {
			logger.WarnContext(shutdownCtx, "echo shutdown", "err", err)
			if err := e.Close(); err != nil {
				logger.WarnContext(shutdownCtx, "echo close after failed shutdown", "err", err)
			}
		}
	}()

	logger.InfoContext(ctx, "http server listening", "address", cfg.HTTP.Address)
	if err := e.Start(cfg.HTTP.Address); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
