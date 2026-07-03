package app

import (
	"google.golang.org/protobuf/encoding/protojson"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"map-service/internal/domain/postgres"
	"map-service/internal/domain/valkey"
	"map-service/internal/infra/config"
	"map-service/internal/infra/docs"
	"map-service/internal/infra/telemetry"
	"map-service/internal/presence"
	mapv1 "map-service/pkg/api/map-service/v1"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
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

// Run assembles the service (telemetry → stores → server → janitor → transport),
// serves gRPC + the HTTP gateway (REST + /metrics + /swagger/), and blocks until
// ctx is cancelled, then shuts everything down gracefully.
func (a *App) Run(ctx context.Context) error {
	// 1. Telemetry first: gRPC client/server handlers capture the global provider.
	tel, err := telemetry.Setup(ctx, "map")
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	defer tel.Shutdown(context.Background())

	// 2. Backing stores.
	pg, err := postgres.NewStore(ctx, a.config.Postgres.DSN)
	if err != nil {
		return fmt.Errorf("postgres: %w", err)
	}
	defer pg.Close()

	vk, err := valkey.NewStore(a.config.Valkey.Address)
	if err != nil {
		return fmt.Errorf("valkey: %w", err)
	}
	defer vk.Close()

	// 3. RPC implementation. The Valkey store is both the presence store and the
	//    session/token resolver (it reads session:{token} that Auth owns).
	presenceTTL := time.Duration(a.config.Map.PresenceTTLSeconds) * time.Second
	srv := NewServer(pg, vk, vk, a.config.Map.RadiusMeters, presenceTTL)

	// 4. Presence janitor: keeps visitor counts honest as presence keys expire.
	janitor := presence.NewJanitor(vk, time.Duration(a.config.Map.JanitorIntervalSeconds)*time.Second)
	janitorCtx, stopJanitor := context.WithCancel(context.Background())
	var janitorWg sync.WaitGroup
	janitorWg.Add(1)
	go func() {
		defer janitorWg.Done()
		janitor.Run(janitorCtx)
	}()
	// Always stop and drain the janitor on any return path.
	defer func() {
		stopJanitor()
		janitorWg.Wait()
	}()

	// 5. gRPC server. forwardAuthToken promotes the incoming auth_token metadata
	//    key (set by the gateway) so the server can resolve the acting user.
	grpcAddr := fmt.Sprintf(":%s", a.config.Service.GrpcPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("grpc listener: %w", err)
	}
	a.grpcServer = grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	reflection.Register(a.grpcServer)
	mapv1.RegisterMapServiceServer(a.grpcServer, srv)

	// 6. HTTP edge: REST gateway + metrics + swagger.
	httpHandler, err := a.httpHandler(ctx, tel)
	if err != nil {
		return err
	}
	a.httpServer = &http.Server{
		Addr:    fmt.Sprintf(":%s", a.config.Service.HttpPort),
		Handler: httpHandler,
	}

	// 7. Serve both; first error or ctx cancellation wins.
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
// plus the /metrics and /swagger/ endpoints.
func (a *App) httpHandler(ctx context.Context, tel *telemetry.Provider) (http.Handler, error) {
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),runtime.WithMetadata(forwardAuthToken))
	dialAddr := fmt.Sprintf("%s:%s", a.config.Service.Host, a.config.Service.GrpcPort)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
	if err := mapv1.RegisterMapServiceHandlerFromEndpoint(ctx, gwMux, dialAddr, opts); err != nil {
		return nil, fmt.Errorf("register gateway: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", tel.MetricsHandler())
	mux.Handle("/swagger/", http.StripPrefix("/swagger", docs.Handler()))
	mux.Handle("/", gwMux)
	return mux, nil
}

// forwardAuthToken copies the incoming `auth_token` HTTP header into gRPC
// metadata under the same key so map_server.callerID can resolve the caller.
func forwardAuthToken(_ context.Context, r *http.Request) metadata.MD {
	if tok := r.Header.Get("auth_token"); tok != "" {
		return metadata.Pairs(authTokenHeader, tok)
	}
	return nil
}

func (a *App) Config() config.Config { return *a.config }
