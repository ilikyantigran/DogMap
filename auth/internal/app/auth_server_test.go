package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"auth-service/internal/domain/password"
	"auth-service/internal/domain/postgres"
	authv1 "auth-service/pkg/api/auth/v1"
	profilesv1 "auth-service/pkg/api/profiles/v1"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

// --- fakes -------------------------------------------------------------------

type fakeCreds struct {
	byLogin map[string]postgres.Credential
	byEmail map[string]postgres.Credential
	insertErr error
}

func newFakeCreds() *fakeCreds {
	return &fakeCreds{byLogin: map[string]postgres.Credential{}, byEmail: map[string]postgres.Credential{}}
}

func (f *fakeCreds) InsertCredential(_ context.Context, c postgres.Credential) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	if _, ok := f.byLogin[c.Login]; ok {
		return postgres.ErrDuplicate
	}
	if _, ok := f.byEmail[c.Email]; ok {
		return postgres.ErrDuplicate
	}
	f.byLogin[c.Login] = c
	f.byEmail[c.Email] = c
	return nil
}

func (f *fakeCreds) FindByLoginOrEmail(_ context.Context, login, email string) (postgres.Credential, error) {
	if login != "" {
		if c, ok := f.byLogin[login]; ok {
			return c, nil
		}
	}
	if email != "" {
		if c, ok := f.byEmail[email]; ok {
			return c, nil
		}
	}
	return postgres.Credential{}, postgres.ErrNotFound
}

type fakeSessions struct {
	created map[string]string // token -> user_id
	deleted []string
	counter int
}

func newFakeSessions() *fakeSessions { return &fakeSessions{created: map[string]string{}} }

func (f *fakeSessions) CreateSession(_ context.Context, userID string, _ time.Duration) (string, error) {
	f.counter++
	token := "tok-" + userID
	f.created[token] = userID
	return token, nil
}
func (f *fakeSessions) DeleteSession(_ context.Context, token string) error {
	f.deleted = append(f.deleted, token)
	delete(f.created, token)
	return nil
}

type fakeProfiles struct {
	calls []*profilesv1.CreateProfileRequest
	err   error
}

func (f *fakeProfiles) CreateProfile(_ context.Context, in *profilesv1.CreateProfileRequest, _ ...grpc.CallOption) (*profilesv1.CreateProfileResponse, error) {
	f.calls = append(f.calls, in)
	if f.err != nil {
		return nil, f.err
	}
	return &profilesv1.CreateProfileResponse{Code: 0, Message: "ok"}, nil
}

func newServer(t *testing.T) (*Server, *fakeCreds, *fakeSessions, *fakeProfiles) {
	t.Helper()
	creds := newFakeCreds()
	sessions := newFakeSessions()
	profiles := &fakeProfiles{}
	srv := NewServer(creds, sessions, password.NewHasher(password.Params{Iterations: 1, Memory: 8 * 1024}), profiles, time.Hour)
	return srv, creds, sessions, profiles
}

// --- Register ----------------------------------------------------------------

func TestRegister_Success_HashesAndSeedsProfile(t *testing.T) {
	srv, creds, _, profiles := newServer(t)
	resp, err := srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "Test1", Email: "Test1@Example.com", Password: "hunter2hunter2",
	})
	if err != nil {
		t.Fatalf("unexpected transport err: %v", err)
	}
	if resp.Code != codeOK || resp.UserId == "" {
		t.Fatalf("expected ok+user_id, got %+v", resp)
	}
	// stored credential must be hashed (Argon2id), not plaintext
	c := creds.byLogin["Test1"]
	if c.PasswordHash == "hunter2hunter2" || c.PasswordHash == "" {
		t.Fatalf("password not hashed: %q", c.PasswordHash)
	}
	if err := password.Verify("hunter2hunter2", c.PasswordHash); err != nil {
		t.Fatalf("stored hash should verify: %v", err)
	}
	// email normalized to lowercase
	if c.Email != "test1@example.com" {
		t.Fatalf("email should be lowercased, got %q", c.Email)
	}
	// profile handoff called once with the same user_id
	if len(profiles.calls) != 1 {
		t.Fatalf("expected 1 CreateProfile call, got %d", len(profiles.calls))
	}
	if profiles.calls[0].GetUserId() != resp.UserId {
		t.Fatalf("handoff user_id mismatch: %s vs %s", profiles.calls[0].GetUserId(), resp.UserId)
	}
}

func TestRegister_DuplicateLogin_NoFieldHint(t *testing.T) {
	srv, _, _, _ := newServer(t)
	req := &authv1.RegisterRequest{Login: "dup", Email: "a@x.com", Password: "password123"}
	if _, err := srv.Register(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	resp, _ := srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "dup", Email: "b@x.com", Password: "password123",
	})
	if resp.Code != codeAlreadyTaken {
		t.Fatalf("expected codeAlreadyTaken, got %+v", resp)
	}
	if resp.UserId != "" {
		t.Fatal("no user_id on duplicate")
	}
}

func TestRegister_MissingFields(t *testing.T) {
	srv, _, _, _ := newServer(t)
	resp, _ := srv.Register(context.Background(), &authv1.RegisterRequest{Login: "", Email: "", Password: ""})
	if resp.Code != codeBadRequest {
		t.Fatalf("expected codeBadRequest, got %+v", resp)
	}
}

func TestRegister_ProfileHandoffFailure_ReportsRetryable(t *testing.T) {
	srv, _, _, profiles := newServer(t)
	profiles.err = errors.New("profiles down")
	resp, _ := srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "x", Email: "x@x.com", Password: "password123",
	})
	if resp.Code != codeInternal {
		t.Fatalf("expected codeInternal on handoff failure, got %+v", resp)
	}
}

// --- Login -------------------------------------------------------------------

func registerUser(t *testing.T, srv *Server, login, email, pass string) {
	t.Helper()
	resp, err := srv.Register(context.Background(), &authv1.RegisterRequest{Login: login, Email: email, Password: pass})
	if err != nil || resp.Code != codeOK {
		t.Fatalf("setup register failed: %+v %v", resp, err)
	}
}

func TestLogin_ByLogin_Success(t *testing.T) {
	srv, _, _, _ := newServer(t)
	registerUser(t, srv, "Test1", "t1@x.com", "password123")
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "Test1", Password: "password123"})
	if resp.Code != codeOK || resp.Token == "" || resp.UserId == "" {
		t.Fatalf("expected ok+token+user_id, got %+v", resp)
	}
}

func TestLogin_ByEmail_Success(t *testing.T) {
	srv, _, _, _ := newServer(t)
	registerUser(t, srv, "Test1", "t1@x.com", "password123")
	// email lookup is case-insensitive because Login lowercases before lookup
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Email: "T1@X.com", Password: "password123"})
	if resp.Code != codeOK || resp.Token == "" {
		t.Fatalf("expected ok+token, got %+v", resp)
	}
}

func TestLogin_WrongPassword_GenericNoToken(t *testing.T) {
	srv, _, _, _ := newServer(t)
	registerUser(t, srv, "Test1", "t1@x.com", "password123")
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "Test1", Password: "wrong"})
	if resp.Code != codeBadCreds {
		t.Fatalf("expected codeBadCreds, got %+v", resp)
	}
	if resp.Token != "" {
		t.Fatal("no token on failed login")
	}
}

func TestLogin_UnknownUser_SameResponseAsWrongPassword(t *testing.T) {
	srv, _, _, _ := newServer(t)
	registerUser(t, srv, "Test1", "t1@x.com", "password123")
	wrongPass, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "Test1", Password: "wrong"})
	noUser, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "ghost", Password: "whatever12"})
	// No enumeration oracle: identical code + message whether the user exists or not.
	if wrongPass.Code != noUser.Code || wrongPass.Message != noUser.Message {
		t.Fatalf("responses must be indistinguishable: %+v vs %+v", wrongPass, noUser)
	}
}

func TestLogin_MissingIdentifier(t *testing.T) {
	srv, _, _, _ := newServer(t)
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Password: "password123"})
	if resp.Code != codeBadRequest {
		t.Fatalf("expected codeBadRequest, got %+v", resp)
	}
}

// --- Logout ------------------------------------------------------------------

func TestLogout_DeletesTokenFromHeader(t *testing.T) {
	srv, _, sessions, _ := newServer(t)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(authTokenHeader, "tok-abc"))
	resp, _ := srv.Logout(ctx, &authv1.LogoutRequest{})
	if resp.Code != codeOK {
		t.Fatalf("expected ok, got %+v", resp)
	}
	if len(sessions.deleted) != 1 || sessions.deleted[0] != "tok-abc" {
		t.Fatalf("expected delete of tok-abc, got %v", sessions.deleted)
	}
}

func TestLogout_NoToken(t *testing.T) {
	srv, _, _, _ := newServer(t)
	resp, _ := srv.Logout(context.Background(), &authv1.LogoutRequest{})
	if resp.Code != codeNoSession {
		t.Fatalf("expected codeNoSession, got %+v", resp)
	}
}

// Path 1 acceptance start: register -> login yields a usable token.
func TestAcceptancePath1_RegisterThenLogin(t *testing.T) {
	srv, _, _, _ := newServer(t)
	reg, _ := srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "Walker", Email: "walker@dogmap.app", Password: "brunothepoodle",
	})
	if reg.Code != codeOK {
		t.Fatalf("register: %+v", reg)
	}
	login, _ := srv.Login(context.Background(), &authv1.LoginRequest{Email: "walker@dogmap.app", Password: "brunothepoodle"})
	if login.Code != codeOK || login.Token == "" {
		t.Fatalf("login: %+v", login)
	}
	if login.UserId != reg.UserId {
		t.Fatalf("login should resolve to the registered user: %s vs %s", login.UserId, reg.UserId)
	}
}
