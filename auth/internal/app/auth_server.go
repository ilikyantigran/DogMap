package app

import (
	"context"
	"errors"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"auth-service/internal/domain/email"
	"auth-service/internal/domain/password"
	"auth-service/internal/domain/postgres"
	authv1 "auth-service/pkg/api/auth/v1"
	profilesv1 "auth-service/pkg/api/profiles/v1"

	"github.com/google/uuid"
	"google.golang.org/grpc/metadata"
)

// authTokenHeader is the metadata key the opaque session token arrives on. The
// HTTP gateway maps the `auth_token` request header into gRPC metadata under
// this key (see app.go metadata annotator).
const authTokenHeader = "auth_token"

// Error codes carried in the response envelope (code:0 == success). These are the
// application-level codes the frontend switches on; they're distinct from gRPC
// status codes.
const (
	codeOK               = 0
	codeAlreadyTaken     = 1 // login or email already registered
	codeBadRequest       = 2 // missing/invalid input
	codeBadCreds         = 3 // login/email + password did not match (no field hint)
	codeNoSession        = 4 // logout without a valid session token
	codeInternal         = 5 // unexpected server error
	codeEmailNotVerified = 6 // correct password but the email is not yet confirmed
	codeBadToken         = 7 // unknown/expired email-verification token
)

// --- narrow store/client interfaces so the server unit-tests against fakes ---

// CredentialStore is the slice of the Postgres store the server needs.
type CredentialStore interface {
	InsertCredential(ctx context.Context, c postgres.Credential) error
	FindByLoginOrEmail(ctx context.Context, login, email string) (postgres.Credential, error)
	FindByEmail(ctx context.Context, email string) (postgres.Credential, error)
	MarkEmailVerified(ctx context.Context, userID string) error
}

// SessionStore is the slice of the Valkey store the server needs.
type SessionStore interface {
	CreateSession(ctx context.Context, userID string, ttl time.Duration) (string, error)
	DeleteSession(ctx context.Context, token string) error
}

// VerifyStore owns the single-use email-verification tokens (verify:{token}).
type VerifyStore interface {
	CreateVerifyToken(ctx context.Context, userID string, ttl time.Duration) (string, error)
	ConsumeVerifyToken(ctx context.Context, token string) (userID string, ok bool, err error)
}

// PasswordHasher hashes registration passwords. (Verify is a package function.)
type PasswordHasher interface {
	Hash(password string) (string, error)
}

// Server implements the generated authv1.AuthServer.
type Server struct {
	authv1.UnimplementedAuthServer

	creds      CredentialStore
	sessions   SessionStore
	hasher     PasswordHasher
	profiles   profilesv1.ProfilesServiceClient
	verify     VerifyStore
	mailer     email.Sender
	appBaseURL string
	sessionTTL time.Duration
	verifyTTL  time.Duration
}

func NewServer(
	creds CredentialStore,
	sessions SessionStore,
	hasher PasswordHasher,
	profiles profilesv1.ProfilesServiceClient,
	verify VerifyStore,
	mailer email.Sender,
	appBaseURL string,
	sessionTTL time.Duration,
	verifyTTL time.Duration,
) *Server {
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	if verifyTTL <= 0 {
		verifyTTL = 24 * time.Hour
	}
	return &Server{
		creds:      creds,
		sessions:   sessions,
		hasher:     hasher,
		profiles:   profiles,
		verify:     verify,
		mailer:     mailer,
		appBaseURL: strings.TrimRight(appBaseURL, "/"),
		sessionTTL: sessionTTL,
		verifyTTL:  verifyTTL,
	}
}

// Register creates a new user: validates input, rejects duplicate login/email,
// hashes the password with Argon2id, persists the credential, then seeds an empty
// profile in Profiles (idempotent handoff). Returns the new string-UUID user id.
func (s *Server) Register(ctx context.Context, in *authv1.RegisterRequest) (*authv1.RegisterResponse, error) {
	login := strings.TrimSpace(in.GetLogin())
	email := strings.TrimSpace(strings.ToLower(in.GetEmail()))
	pass := in.GetPassword()

	if login == "" || email == "" || pass == "" {
		return &authv1.RegisterResponse{Code: codeBadRequest, Message: "login, email and password are required"}, nil
	}

	hash, err := s.hasher.Hash(pass)
	if err != nil {
		slog.ErrorContext(ctx, "hash password", "err", err)
		return &authv1.RegisterResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	userID := uuid.NewString()
	err = s.creds.InsertCredential(ctx, postgres.Credential{
		UserID:       userID,
		Login:        login,
		Email:        email,
		PasswordHash: hash,
	})
	switch {
	case errors.Is(err, postgres.ErrDuplicate):
		// Do not reveal which of login/email collided.
		return &authv1.RegisterResponse{Code: codeAlreadyTaken, Message: "login or email already registered"}, nil
	case err != nil:
		slog.ErrorContext(ctx, "insert credential", "err", err)
		return &authv1.RegisterResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	// Handoff: seed an empty profile in Profiles. Synchronous + idempotent so it's
	// retry-safe. A failure here does not roll back the credential — the profile
	// can be re-created on retry keyed by the same user_id.
	if _, err := s.profiles.CreateProfile(ctx, &profilesv1.CreateProfileRequest{
		UserId: userID,
		Login:  login,
		Email:  email,
	}); err != nil {
		slog.ErrorContext(ctx, "create profile handoff failed", "user_id", userID, "err", err)
		return &authv1.RegisterResponse{Code: codeInternal, Message: "profile creation failed, please retry"}, nil
	}

	// Send the confirmation email. A failure here does NOT fail registration: the
	// account already exists (unverified) and the user can trigger Resend. We only
	// log it. Best-effort delivery keeps a flaky SMTP server from stranding signups.
	s.sendVerification(ctx, userID, email)

	return &authv1.RegisterResponse{Code: codeOK, Message: "ok", UserId: userID}, nil
}

// sendVerification mints a single-use verification token and emails the
// ${app_base_url}/verify?token=... link. Errors are logged, never returned —
// callers treat email delivery as best-effort (Resend covers failures).
func (s *Server) sendVerification(ctx context.Context, userID, email string) {
	token, err := s.verify.CreateVerifyToken(ctx, userID, s.verifyTTL)
	if err != nil {
		slog.ErrorContext(ctx, "create verify token", "user_id", userID, "err", err)
		return
	}
	link := s.appBaseURL + "/verify?token=" + url.QueryEscape(token)
	if err := s.mailer.SendVerification(ctx, email, link); err != nil {
		slog.ErrorContext(ctx, "send verification email", "user_id", userID, "err", err)
	}
}

// Login authenticates with (login OR email) AND password and, on success, issues
// an opaque session token stored in Valkey. On any failure it returns a generic
// bad-credentials envelope with no token and no hint about which field was wrong
// (no account-enumeration oracle).
func (s *Server) Login(ctx context.Context, in *authv1.LoginRequest) (*authv1.LoginResponse, error) {
	login := strings.TrimSpace(in.GetLogin())
	email := strings.TrimSpace(strings.ToLower(in.GetEmail()))
	pass := in.GetPassword()

	if (login == "" && email == "") || pass == "" {
		return &authv1.LoginResponse{Code: codeBadRequest, Message: "provide login or email, and a password"}, nil
	}

	cred, err := s.creds.FindByLoginOrEmail(ctx, login, email)
	if err != nil {
		if !errors.Is(err, postgres.ErrNotFound) {
			slog.ErrorContext(ctx, "find credential", "err", err)
		}
		// Same generic response whether the user is missing or the password is wrong.
		return badCreds(), nil
	}

	if err := password.Verify(pass, cred.PasswordHash); err != nil {
		return badCreds(), nil
	}

	// Email-verification gate. Checked ONLY after the password verifies, so the
	// not-verified state is never a bare account-enumeration oracle: a caller must
	// already know the password to distinguish "unverified" from "bad creds".
	if !cred.EmailVerified {
		return &authv1.LoginResponse{
			Code:    codeEmailNotVerified,
			Message: "please confirm your email before logging in",
		}, nil
	}

	token, err := s.sessions.CreateSession(ctx, cred.UserID, s.sessionTTL)
	if err != nil {
		slog.ErrorContext(ctx, "create session", "err", err)
		return &authv1.LoginResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	return &authv1.LoginResponse{Code: codeOK, Message: "ok", Token: token, UserId: cred.UserID}, nil
}

// Logout revokes the caller's session. The token is read from the auth_token
// header via gRPC metadata — never from the body. Deleting the key makes the
// token instantly unusable. Idempotent: a missing/empty token still returns ok
// on the delete but a truly empty token is reported so the client knows it sent
// no credential.
func (s *Server) Logout(ctx context.Context, _ *authv1.LogoutRequest) (*authv1.LogoutResponse, error) {
	token := tokenFromContext(ctx)
	if token == "" {
		return &authv1.LogoutResponse{Code: codeNoSession, Message: "missing auth_token"}, nil
	}
	if err := s.sessions.DeleteSession(ctx, token); err != nil {
		slog.ErrorContext(ctx, "delete session", "err", err)
		return &authv1.LogoutResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	return &authv1.LogoutResponse{Code: codeOK, Message: "ok"}, nil
}

// VerifyEmail confirms a registration by its single-use token. It consumes the
// token (deleting it) and marks the account verified. Unknown/expired tokens
// return codeBadToken and change nothing. Idempotent semantics: a fresh valid
// token flips the flag; a re-used token is simply "invalid" since it's gone.
func (s *Server) VerifyEmail(ctx context.Context, in *authv1.VerifyEmailRequest) (*authv1.VerifyEmailResponse, error) {
	token := strings.TrimSpace(in.GetToken())
	if token == "" {
		return &authv1.VerifyEmailResponse{Code: codeBadRequest, Message: "token is required"}, nil
	}

	userID, ok, err := s.verify.ConsumeVerifyToken(ctx, token)
	if err != nil {
		slog.ErrorContext(ctx, "consume verify token", "err", err)
		return &authv1.VerifyEmailResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	if !ok {
		return &authv1.VerifyEmailResponse{Code: codeBadToken, Message: "invalid or expired verification link"}, nil
	}

	if err := s.creds.MarkEmailVerified(ctx, userID); err != nil {
		slog.ErrorContext(ctx, "mark email verified", "user_id", userID, "err", err)
		return &authv1.VerifyEmailResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	return &authv1.VerifyEmailResponse{Code: codeOK, Message: "email confirmed"}, nil
}

// ResendVerification re-sends the confirmation email for an unverified account.
// It ALWAYS returns the same generic ok regardless of whether the email exists
// or is already verified — no account-enumeration oracle. A new email is only
// actually sent when the address maps to an existing, still-unverified account.
func (s *Server) ResendVerification(ctx context.Context, in *authv1.ResendVerificationRequest) (*authv1.ResendVerificationResponse, error) {
	email := strings.TrimSpace(strings.ToLower(in.GetEmail()))
	generic := &authv1.ResendVerificationResponse{
		Code:    codeOK,
		Message: "if that email needs confirming, we've sent a new link",
	}
	if email == "" {
		return &authv1.ResendVerificationResponse{Code: codeBadRequest, Message: "email is required"}, nil
	}

	cred, err := s.creds.FindByEmail(ctx, email)
	if err != nil {
		if !errors.Is(err, postgres.ErrNotFound) {
			slog.ErrorContext(ctx, "find by email", "err", err)
		}
		return generic, nil // unknown email → same generic response, no send
	}
	if cred.EmailVerified {
		return generic, nil // already verified → nothing to send, no leak
	}

	s.sendVerification(ctx, cred.UserID, cred.Email)
	return generic, nil
}

func badCreds() *authv1.LoginResponse {
	return &authv1.LoginResponse{Code: codeBadCreds, Message: "invalid credentials"}
}

// tokenFromContext extracts the opaque session token from the auth_token gRPC
// metadata header. This is the acting-identity source — bodies never carry it.
func tokenFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if vals := md.Get(authTokenHeader); len(vals) > 0 {
		return strings.TrimSpace(vals[0])
	}
	return ""
}
