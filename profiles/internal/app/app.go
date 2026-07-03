package app

import (
	"google.golang.org/protobuf/encoding/protojson"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"profiles/internal/domain/postgres"
	"profiles/internal/domain/valkey"
	"profiles/internal/infra/config"
	"profiles/internal/infra/docs"
	"profiles/internal/infra/telemetry"
	profilesv1 "profiles/pkg/api/profiles/v1"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
)

// App is the service's self-contained runtime. main builds it and calls Run;
// this is the single place that knows how to assemble and shut down the service.
type App struct {
	config     *config.Config
	grpcServer *grpc.Server
	httpServer *http.Server
}

// NewApp loads config only — cheap, no connections to dependencies yet.
func NewApp(path string) (*App, error) {
	cfg, err := config.InitConfig(path)
	if err != nil {
		return nil, err
	}
	return &App{config: cfg}, nil
}

// Run assembles the service (telemetry → stores → server → transport), serves
// gRPC + the HTTP gateway (REST + /metrics + /swagger/), and blocks until ctx is
// cancelled, then shuts everything down gracefully.
func (a *App) Run(ctx context.Context) error {
	tel, err := telemetry.Setup(ctx, "profiles")
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	defer tel.Shutdown(context.Background())

	// Backing stores: Postgres owns the `profiles` schema; Valkey owns the
	// friends:{uid} cache and resolves session tokens (session:* owned by Auth).
	pgStore, err := postgres.NewStore(ctx, a.config.Postgres.DSN)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pgStore.Close()

	cache, err := valkey.NewStore(a.config.Valkey.Address)
	if err != nil {
		return fmt.Errorf("valkey: %w", err)
	}
	defer cache.Close()

	srv := NewServer(pgStore, cache)

	// gRPC server.
	grpcAddr := fmt.Sprintf(":%s", a.config.Service.GrpcPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("grpc listener: %w", err)
	}
	a.grpcServer = grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	reflection.Register(a.grpcServer)
	profilesv1.RegisterProfilesServiceServer(a.grpcServer, srv)

	// HTTP edge: REST gateway + metrics + swagger.
	httpHandler, err := a.httpHandler(ctx, tel)
	if err != nil {
		return err
	}
	a.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%s", a.config.Service.HttpPort),
		Handler: httpHandler,
	}

	errCh := make(chan error, 2)
	go func() {
		slog.Info("gRPC listening", "addr", grpcAddr)
		errCh <- a.grpcServer.Serve(lis)
	}()
	go func() {
		slog.Info("HTTP listening (REST gateway, /swagger/, /metrics)", "addr", a.httpServer.Addr)
		if err := a.httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutdown signal received, stopping servers")
		a.grpcServer.GracefulStop()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := a.httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("http shutdown: %w", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// httpHandler builds the REST gateway (dialing this service's own gRPC server)
// plus /metrics and /swagger/. The gateway forwards the `auth_token` header to
// gRPC metadata so the server can resolve the acting user.
func (a *App) httpHandler(ctx context.Context, tel *telemetry.Provider) (http.Handler, error) {
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			if key == "Auth_token" || key == "Auth-Token" || key == "auth_token" {
				return "auth_token", true
			}
			return runtime.DefaultHeaderMatcher(key)
		}),
	)
	dialAddr := fmt.Sprintf("%s:%s", a.config.Service.Host, a.config.Service.GrpcPort)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
	if err := profilesv1.RegisterProfilesServiceHandlerFromEndpoint(ctx, gwMux, dialAddr, opts); err != nil {
		return nil, fmt.Errorf("register gateway: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", tel.MetricsHandler())
	mux.Handle("/swagger/", http.StripPrefix("/swagger", docs.Handler()))
	mux.Handle("/", gwMux)
	return mux, nil
}

func (a *App) Config() config.Config { return *a.config }
