package app

import (
	"context"
	"errors"
	"fmt"
	"google.golang.org/protobuf/encoding/protojson"
	"log/slog"
	"net"
	"net/http"
	"time"

	"auth-service/internal/clients"
	"auth-service/internal/domain/email"
	"auth-service/internal/domain/password"
	"auth-service/internal/domain/postgres"
	"auth-service/internal/domain/valkey"
	"auth-service/internal/infra/config"
	"auth-service/internal/infra/docs"
	"auth-service/internal/infra/telemetry"
	authv1 "auth-service/pkg/api/auth/v1"

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

// Run assembles the service (telemetry → clients → stores → server → transport),
// serves gRPC + the HTTP gateway (REST + /metrics + /swagger/), and blocks until
// ctx is cancelled, then shuts everything down gracefully.
func (a *App) Run(ctx context.Context) error {
	// 1. Telemetry first: gRPC client/server handlers capture the global provider.
	tel, err := telemetry.Setup(ctx, "auth")
	if err != nil {
		return fmt.Errorf("telemetry: %w", err)
	}
	defer tel.Shutdown(context.Background())

	// 2. Downstream clients: Profiles (CreateProfile handoff on register).
	cl, err := clients.Dial(a.config.Downstreams.Profiles)
	if err != nil {
		return fmt.Errorf("clients: %w", err)
	}
	defer cl.Close()

	// 3. Backing stores: Postgres (credentials) + Valkey (sessions).
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

	// 4. Construct the RPC implementation.
	hasher := password.NewHasher(password.Params{
		Memory:      a.config.Auth.Argon2Memory,
		Iterations:  a.config.Auth.Argon2Iterations,
		Parallelism: a.config.Auth.Argon2Parallelism,
	})
	ttl := time.Duration(a.config.Auth.SessionTTLSeconds) * time.Second
	verifyTTL := time.Duration(a.config.Auth.VerifyTTLSeconds) * time.Second

	// Mailer: SMTP when a host is configured (Docker → Mailpit), else a no-op
	// sender that logs the link so local `go run` works without a mail server.
	var mailer email.Sender
	if a.config.SMTP.Host != "" {
		mailer = email.NewSMTPSender(a.config.SMTP.Host, a.config.SMTP.Port, a.config.SMTP.From)
		slog.Info("email: SMTP sender", "host", a.config.SMTP.Host, "port", a.config.SMTP.Port)
	} else {
		mailer = email.NoopSender{}
		slog.Warn("email: no SMTP host configured, using no-op sender (links are logged only)")
	}

	srv := NewServer(pg, vk, hasher, cl.Profiles, vk, mailer, a.config.AppBaseURL, ttl, verifyTTL)

	// 5. gRPC server.
	grpcAddr := fmt.Sprintf(":%s", a.config.Service.GrpcPort)
	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		return fmt.Errorf("grpc listener: %w", err)
	}
	a.grpcServer = grpc.NewServer(grpc.StatsHandler(otelgrpc.NewServerHandler()))
	reflection.Register(a.grpcServer)
	authv1.RegisterAuthServer(a.grpcServer, srv)

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
// plus the /metrics and /swagger/ endpoints. It installs a metadata annotator so
// the `auth_token` request header is forwarded into gRPC metadata — that's how
// the acting identity reaches Logout without ever appearing in the body.
func (a *App) httpHandler(ctx context.Context, tel *telemetry.Provider) (http.Handler, error) {
	gwMux := runtime.NewServeMux(
		runtime.WithMarshalerOption(runtime.MIMEWildcard, &runtime.JSONPb{
			MarshalOptions:   protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true},
			UnmarshalOptions: protojson.UnmarshalOptions{DiscardUnknown: true},
		}),
		runtime.WithMetadata(func(_ context.Context, r *http.Request) metadata.MD {
			if tok := r.Header.Get(authTokenHeader); tok != "" {
				return metadata.Pairs(authTokenHeader, tok)
			}
			return nil
		}),
	)
	dialAddr := fmt.Sprintf("%s:%s", a.config.Service.Host, a.config.Service.GrpcPort)
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	}
	if err := authv1.RegisterAuthHandlerFromEndpoint(ctx, gwMux, dialAddr, opts); err != nil {
		return nil, fmt.Errorf("register gateway: %w", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/metrics", tel.MetricsHandler())
	mux.Handle("/swagger/", http.StripPrefix("/swagger", docs.Handler()))
	mux.Handle("/", gwMux)
	return mux, nil
}

func (a *App) Config() config.Config { return *a.config }
