package app

import (
	"context"
	"errors"
	"strings"
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
	byID      map[string]*postgres.Credential // canonical store, keyed by user_id
	insertErr error
}

func newFakeCreds() *fakeCreds {
	return &fakeCreds{byID: map[string]*postgres.Credential{}}
}

func (f *fakeCreds) InsertCredential(_ context.Context, c postgres.Credential) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	for _, existing := range f.byID {
		if existing.Login == c.Login || existing.Email == c.Email {
			return postgres.ErrDuplicate
		}
	}
	cp := c
	f.byID[c.UserID] = &cp
	return nil
}

func (f *fakeCreds) FindByLoginOrEmail(_ context.Context, login, email string) (postgres.Credential, error) {
	for _, c := range f.byID {
		if (login != "" && c.Login == login) || (email != "" && c.Email == email) {
			return *c, nil
		}
	}
	return postgres.Credential{}, postgres.ErrNotFound
}

func (f *fakeCreds) FindByEmail(_ context.Context, email string) (postgres.Credential, error) {
	for _, c := range f.byID {
		if c.Email == email {
			return *c, nil
		}
	}
	return postgres.Credential{}, postgres.ErrNotFound
}

func (f *fakeCreds) MarkEmailVerified(_ context.Context, userID string) error {
	if c, ok := f.byID[userID]; ok {
		c.EmailVerified = true
	}
	return nil
}

// byLogin is a test helper mirroring the old map lookup (case-sensitive on the
// stored login) so existing assertions keep reading naturally.
func (f *fakeCreds) byLoginLookup(login string) postgres.Credential {
	for _, c := range f.byID {
		if c.Login == login {
			return *c
		}
	}
	return postgres.Credential{}
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

// fakeVerify is an in-memory verify:{token} -> user_id store (single-use).
type fakeVerify struct {
	tokens  map[string]string // token -> user_id
	counter int
}

func newFakeVerify() *fakeVerify { return &fakeVerify{tokens: map[string]string{}} }

func (f *fakeVerify) CreateVerifyToken(_ context.Context, userID string, _ time.Duration) (string, error) {
	f.counter++
	token := "vtok-" + userID
	f.tokens[token] = userID
	return token, nil
}

func (f *fakeVerify) ConsumeVerifyToken(_ context.Context, token string) (string, bool, error) {
	userID, ok := f.tokens[token]
	if !ok {
		return "", false, nil
	}
	delete(f.tokens, token) // single-use
	return userID, true, nil
}

// fakeEmail records the verification links it was asked to send.
type fakeEmail struct {
	sent []struct{ to, url string }
	err  error
}

func (f *fakeEmail) SendVerification(_ context.Context, to, verifyURL string) error {
	f.sent = append(f.sent, struct{ to, url string }{to, verifyURL})
	return f.err
}

type serverDeps struct {
	creds    *fakeCreds
	sessions *fakeSessions
	profiles *fakeProfiles
	verify   *fakeVerify
	email    *fakeEmail
}

func newServer(t *testing.T) (*Server, *serverDeps) {
	t.Helper()
	d := &serverDeps{
		creds:    newFakeCreds(),
		sessions: newFakeSessions(),
		profiles: &fakeProfiles{},
		verify:   newFakeVerify(),
		email:    &fakeEmail{},
	}
	srv := NewServer(
		d.creds, d.sessions,
		password.NewHasher(password.Params{Iterations: 1, Memory: 8 * 1024}),
		d.profiles, d.verify, d.email,
		"http://localhost:5173", time.Hour, time.Hour,
	)
	return srv, d
}

// --- Register ----------------------------------------------------------------

func TestRegister_Success_HashesAndSeedsProfile(t *testing.T) {
	srv, d := newServer(t)
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
	c := d.creds.byLoginLookup("Test1")
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
	// account starts UNVERIFIED
	if c.EmailVerified {
		t.Fatal("new account must start unverified")
	}
	// profile handoff called once with the same user_id
	if len(d.profiles.calls) != 1 {
		t.Fatalf("expected 1 CreateProfile call, got %d", len(d.profiles.calls))
	}
	if d.profiles.calls[0].GetUserId() != resp.UserId {
		t.Fatalf("handoff user_id mismatch: %s vs %s", d.profiles.calls[0].GetUserId(), resp.UserId)
	}
}

// Register mints a verification token and emails a /verify?token= link.
func TestRegister_SendsVerificationEmail(t *testing.T) {
	srv, d := newServer(t)
	resp, _ := srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "Test1", Email: "test1@example.com", Password: "password123",
	})
	if resp.Code != codeOK {
		t.Fatalf("register: %+v", resp)
	}
	if len(d.email.sent) != 1 {
		t.Fatalf("expected 1 verification email, got %d", len(d.email.sent))
	}
	got := d.email.sent[0]
	if got.to != "test1@example.com" {
		t.Fatalf("email sent to %q, want test1@example.com", got.to)
	}
	if !strings.Contains(got.url, "http://localhost:5173/verify?token=") {
		t.Fatalf("verify link malformed: %q", got.url)
	}
	// the token in the link must resolve to the registered user
	token := got.url[strings.Index(got.url, "token=")+len("token="):]
	uid, ok, _ := d.verify.ConsumeVerifyToken(context.Background(), token)
	if !ok || uid != resp.UserId {
		t.Fatalf("verify token should map to user %s, got %s ok=%v", resp.UserId, uid, ok)
	}
}

// A failure to send the email must NOT fail registration (account exists; the
// user can Resend). Trade-off accepted in the plan.
func TestRegister_EmailSendFailure_StillSucceeds(t *testing.T) {
	srv, d := newServer(t)
	d.email.err = errors.New("smtp down")
	resp, _ := srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "Test1", Email: "test1@example.com", Password: "password123",
	})
	if resp.Code != codeOK || resp.UserId == "" {
		t.Fatalf("register should still succeed despite email failure, got %+v", resp)
	}
}

func TestRegister_DuplicateLogin_NoFieldHint(t *testing.T) {
	srv, _ := newServer(t)
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
	srv, _ := newServer(t)
	resp, _ := srv.Register(context.Background(), &authv1.RegisterRequest{Login: "", Email: "", Password: ""})
	if resp.Code != codeBadRequest {
		t.Fatalf("expected codeBadRequest, got %+v", resp)
	}
}

func TestRegister_ProfileHandoffFailure_ReportsRetryable(t *testing.T) {
	srv, d := newServer(t)
	d.profiles.err = errors.New("profiles down")
	resp, _ := srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "x", Email: "x@x.com", Password: "password123",
	})
	if resp.Code != codeInternal {
		t.Fatalf("expected codeInternal on handoff failure, got %+v", resp)
	}
}

// --- Login -------------------------------------------------------------------

// registerUser registers and leaves the account UNVERIFIED.
func registerUser(t *testing.T, srv *Server, login, email, pass string) {
	t.Helper()
	resp, err := srv.Register(context.Background(), &authv1.RegisterRequest{Login: login, Email: email, Password: pass})
	if err != nil || resp.Code != codeOK {
		t.Fatalf("setup register failed: %+v %v", resp, err)
	}
}

// registerVerifiedUser registers and then confirms the email via the token the
// registration flow minted, leaving the account ready to log in.
func registerVerifiedUser(t *testing.T, srv *Server, d *serverDeps, login, email, pass string) {
	t.Helper()
	registerUser(t, srv, login, email, pass)
	last := d.email.sent[len(d.email.sent)-1]
	token := last.url[strings.Index(last.url, "token=")+len("token="):]
	resp, err := srv.VerifyEmail(context.Background(), &authv1.VerifyEmailRequest{Token: token})
	if err != nil || resp.Code != codeOK {
		t.Fatalf("setup verify failed: %+v %v", resp, err)
	}
}

func TestLogin_ByLogin_Success(t *testing.T) {
	srv, d := newServer(t)
	registerVerifiedUser(t, srv, d, "Test1", "t1@x.com", "password123")
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "Test1", Password: "password123"})
	if resp.Code != codeOK || resp.Token == "" || resp.UserId == "" {
		t.Fatalf("expected ok+token+user_id, got %+v", resp)
	}
}

func TestLogin_ByEmail_Success(t *testing.T) {
	srv, d := newServer(t)
	registerVerifiedUser(t, srv, d, "Test1", "t1@x.com", "password123")
	// email lookup is case-insensitive because Login lowercases before lookup
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Email: "T1@X.com", Password: "password123"})
	if resp.Code != codeOK || resp.Token == "" {
		t.Fatalf("expected ok+token, got %+v", resp)
	}
}

// An unverified account with the CORRECT password is rejected with the distinct
// not-verified code and no token (checked AFTER password verify → no oracle).
func TestLogin_Unverified_RejectedNoToken(t *testing.T) {
	srv, _ := newServer(t)
	registerUser(t, srv, "Test1", "t1@x.com", "password123")
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "Test1", Password: "password123"})
	if resp.Code != codeEmailNotVerified {
		t.Fatalf("expected codeEmailNotVerified, got %+v", resp)
	}
	if resp.Token != "" {
		t.Fatal("no token for an unverified account")
	}
}

// The unverified state is only reachable with the right password: a wrong
// password on an unverified account still yields generic bad-creds, so the
// not-verified code can't be used to enumerate accounts.
func TestLogin_Unverified_WrongPassword_GenericNoOracle(t *testing.T) {
	srv, _ := newServer(t)
	registerUser(t, srv, "Test1", "t1@x.com", "password123")
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "Test1", Password: "wrong"})
	if resp.Code != codeBadCreds {
		t.Fatalf("expected codeBadCreds for wrong password, got %+v", resp)
	}
}

func TestLogin_WrongPassword_GenericNoToken(t *testing.T) {
	srv, d := newServer(t)
	registerVerifiedUser(t, srv, d, "Test1", "t1@x.com", "password123")
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "Test1", Password: "wrong"})
	if resp.Code != codeBadCreds {
		t.Fatalf("expected codeBadCreds, got %+v", resp)
	}
	if resp.Token != "" {
		t.Fatal("no token on failed login")
	}
}

func TestLogin_UnknownUser_SameResponseAsWrongPassword(t *testing.T) {
	srv, d := newServer(t)
	registerVerifiedUser(t, srv, d, "Test1", "t1@x.com", "password123")
	wrongPass, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "Test1", Password: "wrong"})
	noUser, _ := srv.Login(context.Background(), &authv1.LoginRequest{Login: "ghost", Password: "whatever12"})
	// No enumeration oracle: identical code + message whether the user exists or not.
	if wrongPass.Code != noUser.Code || wrongPass.Message != noUser.Message {
		t.Fatalf("responses must be indistinguishable: %+v vs %+v", wrongPass, noUser)
	}
}

func TestLogin_MissingIdentifier(t *testing.T) {
	srv, _ := newServer(t)
	resp, _ := srv.Login(context.Background(), &authv1.LoginRequest{Password: "password123"})
	if resp.Code != codeBadRequest {
		t.Fatalf("expected codeBadRequest, got %+v", resp)
	}
}

// --- VerifyEmail -------------------------------------------------------------

func TestVerifyEmail_MarksVerifiedAndBurnsToken(t *testing.T) {
	srv, d := newServer(t)
	srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "Test1", Email: "t1@x.com", Password: "password123",
	})
	// pull the token straight out of the emailed /verify?token= link
	last := d.email.sent[0]
	token := last.url[strings.Index(last.url, "token=")+len("token="):]
	resp, _ := srv.VerifyEmail(context.Background(), &authv1.VerifyEmailRequest{Token: token})
	if resp.Code != codeOK {
		t.Fatalf("expected ok, got %+v", resp)
	}
	if !d.creds.byLoginLookup("Test1").EmailVerified {
		t.Fatal("account should be marked verified")
	}
	// token is single-use: a second verify with the same token fails
	again, _ := srv.VerifyEmail(context.Background(), &authv1.VerifyEmailRequest{Token: token})
	if again.Code == codeOK {
		t.Fatal("verification token must be single-use")
	}
}

func TestVerifyEmail_BadToken(t *testing.T) {
	srv, _ := newServer(t)
	resp, _ := srv.VerifyEmail(context.Background(), &authv1.VerifyEmailRequest{Token: "nope"})
	if resp.Code != codeBadToken {
		t.Fatalf("expected codeBadToken, got %+v", resp)
	}
}

func TestVerifyEmail_EmptyToken(t *testing.T) {
	srv, _ := newServer(t)
	resp, _ := srv.VerifyEmail(context.Background(), &authv1.VerifyEmailRequest{Token: ""})
	if resp.Code != codeBadRequest {
		t.Fatalf("expected codeBadRequest, got %+v", resp)
	}
}

// --- ResendVerification ------------------------------------------------------

func TestResendVerification_UnverifiedEmail_SendsAgain(t *testing.T) {
	srv, d := newServer(t)
	registerUser(t, srv, "Test1", "t1@x.com", "password123")
	before := len(d.email.sent)
	resp, _ := srv.ResendVerification(context.Background(), &authv1.ResendVerificationRequest{Email: "t1@x.com"})
	if resp.Code != codeOK {
		t.Fatalf("expected ok, got %+v", resp)
	}
	if len(d.email.sent) != before+1 {
		t.Fatalf("expected a new email to be sent, sent count %d -> %d", before, len(d.email.sent))
	}
}

// Unknown email returns the SAME generic ok — no enumeration oracle.
func TestResendVerification_UnknownEmail_GenericOkNoSend(t *testing.T) {
	srv, d := newServer(t)
	registerUser(t, srv, "Test1", "t1@x.com", "password123")
	known, _ := srv.ResendVerification(context.Background(), &authv1.ResendVerificationRequest{Email: "t1@x.com"})
	unknown, _ := srv.ResendVerification(context.Background(), &authv1.ResendVerificationRequest{Email: "ghost@x.com"})
	if known.Code != unknown.Code || known.Message != unknown.Message {
		t.Fatalf("responses must be indistinguishable: %+v vs %+v", known, unknown)
	}
	// no email is sent for the unknown address (only the known one)
	for _, s := range d.email.sent {
		if s.to == "ghost@x.com" {
			t.Fatal("must not send to an unknown address")
		}
	}
}

// Already-verified email: no new email, still generic ok.
func TestResendVerification_AlreadyVerified_GenericOkNoSend(t *testing.T) {
	srv, d := newServer(t)
	registerVerifiedUser(t, srv, d, "Test1", "t1@x.com", "password123")
	before := len(d.email.sent)
	resp, _ := srv.ResendVerification(context.Background(), &authv1.ResendVerificationRequest{Email: "t1@x.com"})
	if resp.Code != codeOK {
		t.Fatalf("expected generic ok, got %+v", resp)
	}
	if len(d.email.sent) != before {
		t.Fatal("must not resend to an already-verified account")
	}
}

// --- Logout ------------------------------------------------------------------

func TestLogout_DeletesTokenFromHeader(t *testing.T) {
	srv, d := newServer(t)
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs(authTokenHeader, "tok-abc"))
	resp, _ := srv.Logout(ctx, &authv1.LogoutRequest{})
	if resp.Code != codeOK {
		t.Fatalf("expected ok, got %+v", resp)
	}
	if len(d.sessions.deleted) != 1 || d.sessions.deleted[0] != "tok-abc" {
		t.Fatalf("expected delete of tok-abc, got %v", d.sessions.deleted)
	}
}

func TestLogout_NoToken(t *testing.T) {
	srv, _ := newServer(t)
	resp, _ := srv.Logout(context.Background(), &authv1.LogoutRequest{})
	if resp.Code != codeNoSession {
		t.Fatalf("expected codeNoSession, got %+v", resp)
	}
}

// Path 1 acceptance start: register -> verify email -> login yields a usable token.
func TestAcceptancePath1_RegisterVerifyThenLogin(t *testing.T) {
	srv, d := newServer(t)
	reg, _ := srv.Register(context.Background(), &authv1.RegisterRequest{
		Login: "Walker", Email: "walker@dogmap.app", Password: "brunothepoodle",
	})
	if reg.Code != codeOK {
		t.Fatalf("register: %+v", reg)
	}
	// unverified login is blocked
	blocked, _ := srv.Login(context.Background(), &authv1.LoginRequest{Email: "walker@dogmap.app", Password: "brunothepoodle"})
	if blocked.Code != codeEmailNotVerified {
		t.Fatalf("unverified login should be blocked, got %+v", blocked)
	}
	// confirm the email
	link := d.email.sent[0].url
	token := link[strings.Index(link, "token=")+len("token="):]
	if v, _ := srv.VerifyEmail(context.Background(), &authv1.VerifyEmailRequest{Token: token}); v.Code != codeOK {
		t.Fatalf("verify: %+v", v)
	}
	// now login succeeds
	login, _ := srv.Login(context.Background(), &authv1.LoginRequest{Email: "walker@dogmap.app", Password: "brunothepoodle"})
	if login.Code != codeOK || login.Token == "" {
		t.Fatalf("login: %+v", login)
	}
	if login.UserId != reg.UserId {
		t.Fatalf("login should resolve to the registered user: %s vs %s", login.UserId, reg.UserId)
	}
}
