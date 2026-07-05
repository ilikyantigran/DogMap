package app

import (
	"context"
	"strings"
	"testing"

	pg "profiles/internal/domain/postgres"
	profilesv1 "profiles/pkg/api/profiles/v1"

	"google.golang.org/grpc/metadata"
)

// --- fakes ------------------------------------------------------------------

// fakeStore is an in-memory profileStore for unit tests. It models profiles,
// friendships (both directions), pending requests, and blocks.
type fakeStore struct {
	profiles    map[string]*pg.Profile
	friendships map[[2]string]bool          // (a,b) -> friends
	blocks      map[[2]string]bool          // (blocker, blocked)
	requests    map[string]*pg.FriendRequest // id -> pending request

	acceptedCalls int
	createCalls   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		profiles:    map[string]*pg.Profile{},
		friendships: map[[2]string]bool{},
		blocks:      map[[2]string]bool{},
		requests:    map[string]*pg.FriendRequest{},
	}
}

func (f *fakeStore) CreateProfile(_ context.Context, userID, login, email string) error {
	f.createCalls++
	if _, ok := f.profiles[userID]; ok {
		return nil // idempotent
	}
	f.profiles[userID] = &pg.Profile{UserID: userID, Login: login, Email: email}
	return nil
}

func (f *fakeStore) GetProfile(_ context.Context, userID string) (*pg.Profile, error) {
	p, ok := f.profiles[userID]
	if !ok {
		return nil, pg.ErrNotFound
	}
	cp := *p
	return &cp, nil
}

// GetProfileByLogin models Postgres citext: matching is case-insensitive.
func (f *fakeStore) GetProfileByLogin(_ context.Context, login string) (*pg.Profile, error) {
	for _, p := range f.profiles {
		if strings.EqualFold(p.Login, login) {
			cp := *p
			return &cp, nil
		}
	}
	return nil, pg.ErrNotFound
}

func (f *fakeStore) LoginFor(_ context.Context, userID string) (string, error) {
	p, ok := f.profiles[userID]
	if !ok {
		return "", pg.ErrNotFound
	}
	return p.Login, nil
}

func (f *fakeStore) UpdateProfile(_ context.Context, userID, name, surname, phone string, pets []pg.Pet) error {
	p, ok := f.profiles[userID]
	if !ok {
		return pg.ErrNotFound
	}
	p.Name, p.Surname, p.Phone, p.Pets = name, surname, phone, pets
	return nil
}

func (f *fakeStore) AreFriends(_ context.Context, a, b string) (bool, error) {
	return f.friendships[[2]string{a, b}], nil
}

func (f *fakeStore) IsBlockedEitherWay(_ context.Context, a, b string) (bool, error) {
	return f.blocks[[2]string{a, b}] || f.blocks[[2]string{b, a}], nil
}

func (f *fakeStore) HasBlocked(_ context.Context, blocker, blocked string) (bool, error) {
	return f.blocks[[2]string{blocker, blocked}], nil
}

func (f *fakeStore) PendingBetween(_ context.Context, a, b string) (*pg.FriendRequest, error) {
	for _, r := range f.requests {
		if r.Status != "PENDING" {
			continue
		}
		if (r.FromUserID == a && r.ToUserID == b) || (r.FromUserID == b && r.ToUserID == a) {
			return r, nil
		}
	}
	return nil, pg.ErrNotFound
}

func (f *fakeStore) CreateFriendRequest(_ context.Context, from, to string) (string, error) {
	id := "req-" + from + "-" + to
	f.requests[id] = &pg.FriendRequest{ID: id, FromUserID: from, ToUserID: to, Status: "PENDING"}
	return id, nil
}

func (f *fakeStore) GetPendingRequest(_ context.Context, id string) (*pg.FriendRequest, error) {
	r, ok := f.requests[id]
	if !ok || r.Status != "PENDING" {
		return nil, pg.ErrNotFound
	}
	cp := *r
	return &cp, nil
}

func (f *fakeStore) AcceptFriendRequest(_ context.Context, id, a, b string) error {
	f.acceptedCalls++
	if r, ok := f.requests[id]; ok {
		r.Status = "ACCEPTED"
	}
	f.friendships[[2]string{a, b}] = true
	f.friendships[[2]string{b, a}] = true
	return nil
}

func (f *fakeStore) DeclineFriendRequest(_ context.Context, id string) error {
	if r, ok := f.requests[id]; ok {
		r.Status = "DECLINED"
	}
	return nil
}

func (f *fakeStore) RemoveFriendship(_ context.Context, a, b string) error {
	delete(f.friendships, [2]string{a, b})
	delete(f.friendships, [2]string{b, a})
	return nil
}

func (f *fakeStore) Block(_ context.Context, blocker, blocked string) error {
	f.blocks[[2]string{blocker, blocked}] = true
	delete(f.friendships, [2]string{blocker, blocked})
	delete(f.friendships, [2]string{blocked, blocker})
	for _, r := range f.requests {
		if r.Status == "PENDING" &&
			((r.FromUserID == blocker && r.ToUserID == blocked) ||
				(r.FromUserID == blocked && r.ToUserID == blocker)) {
			r.Status = "DECLINED"
		}
	}
	return nil
}

func (f *fakeStore) Unblock(_ context.Context, blocker, blocked string) error {
	delete(f.blocks, [2]string{blocker, blocked})
	return nil
}

func (f *fakeStore) FriendIDs(_ context.Context, userID string) ([]string, error) {
	var ids []string
	for k := range f.friendships {
		if k[0] == userID {
			ids = append(ids, k[1])
		}
	}
	return ids, nil
}

func (f *fakeStore) Friends(_ context.Context, userID string) ([]pg.FriendRef, error) {
	var out []pg.FriendRef
	for k := range f.friendships {
		if k[0] == userID {
			login := ""
			if p, ok := f.profiles[k[1]]; ok {
				login = p.Login
			}
			out = append(out, pg.FriendRef{UserID: k[1], Login: login})
		}
	}
	return out, nil
}

func (f *fakeStore) IncomingPending(_ context.Context, userID string) ([]pg.FriendRequest, map[string]string, error) {
	var out []pg.FriendRequest
	logins := map[string]string{}
	for _, r := range f.requests {
		if r.Status == "PENDING" && r.ToUserID == userID {
			out = append(out, *r)
			if p, ok := f.profiles[r.FromUserID]; ok {
				logins[r.FromUserID] = p.Login
			}
		}
	}
	return out, logins, nil
}

func (f *fakeStore) OutgoingPending(_ context.Context, userID string) ([]pg.FriendRequest, map[string]string, error) {
	var out []pg.FriendRequest
	logins := map[string]string{}
	for _, r := range f.requests {
		if r.Status == "PENDING" && r.FromUserID == userID {
			out = append(out, *r)
			if p, ok := f.profiles[r.ToUserID]; ok {
				logins[r.ToUserID] = p.Login
			}
		}
	}
	return out, logins, nil
}

// fakeCache resolves a fixed token→user map and records friends:{uid} writes.
type fakeCache struct {
	sessions map[string]string   // token -> user_id
	friends  map[string][]string // user_id -> cached friend ids
}

func newFakeCache() *fakeCache {
	return &fakeCache{sessions: map[string]string{}, friends: map[string][]string{}}
}

func (c *fakeCache) ResolveSession(_ context.Context, token string) (string, error) {
	return c.sessions[token], nil
}

func (c *fakeCache) SetFriends(_ context.Context, userID string, ids []string) error {
	c.friends[userID] = ids
	return nil
}

// ctxWithToken builds an incoming-metadata context carrying auth_token.
func ctxWithToken(token string) context.Context {
	md := metadata.New(map[string]string{authHeader: token})
	return metadata.NewIncomingContext(context.Background(), md)
}

// helper: fully-wired fixture with two profiles and a session for each.
func fixture() (*Server, *fakeStore, *fakeCache) {
	store := newFakeStore()
	cache := newFakeCache()
	store.profiles["u1"] = &pg.Profile{UserID: "u1", Login: "Test1", Name: "Ann", Email: "a@x.io", Phone: "+100"}
	store.profiles["u2"] = &pg.Profile{UserID: "u2", Login: "Test2", Name: "Bob", Email: "b@x.io", Phone: "+200"}
	cache.sessions["tok1"] = "u1"
	cache.sessions["tok2"] = "u2"
	return NewServer(store, cache), store, cache
}

// --- identity / auth --------------------------------------------------------

func TestGetUserInfo_NoToken_Unauthorized(t *testing.T) {
	s, _, _ := fixture()
	resp, err := s.GetUserInfo(context.Background(), &profilesv1.GetUserInfoRequest{UserIdTarget: "u1"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Code != codeUnauthorized {
		t.Fatalf("want %d, got %d", codeUnauthorized, resp.Code)
	}
}

func TestEditUser_UsesTokenNotBody(t *testing.T) {
	// EditUser has no user_id in the body; acting user must come from tok2 → u2.
	s, store, _ := fixture()
	resp, err := s.EditUser(ctxWithToken("tok2"), &profilesv1.EditUserRequest{
		Name: "Bobby", Surname: "Jones", Phone: "+999",
		Pets: []*profilesv1.Pet{{Breed: "Poodle", Name: "Bruno", Sex: "M", IsCastrated: true, Age: 3}},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Code != codeOK {
		t.Fatalf("want ok, got code=%d msg=%q", resp.Code, resp.Message)
	}
	if store.profiles["u2"].Name != "Bobby" {
		t.Fatalf("u2 not updated: %+v", store.profiles["u2"])
	}
	if store.profiles["u1"].Name == "Bobby" {
		t.Fatal("u1 was modified — body/token confusion")
	}
	if resp.UserId != "u2" {
		t.Fatalf("acting user should be u2, got %q", resp.UserId)
	}
}

// --- PII scoping (Path 2 privacy) -------------------------------------------

func TestGetUserInfo_NonFriend_ReducedShape(t *testing.T) {
	s, _, _ := fixture()
	// u1 views u2, not friends → reduced: no email/phone.
	resp, err := s.GetUserInfo(ctxWithToken("tok1"), &profilesv1.GetUserInfoRequest{UserIdTarget: "u2"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != codeOK {
		t.Fatalf("want ok, got %d", resp.Code)
	}
	if resp.HasPii {
		t.Fatal("non-friend must get reduced shape (has_pii=false)")
	}
	if resp.Email != "" || resp.Phone != "" {
		t.Fatalf("PII leaked to non-friend: email=%q phone=%q", resp.Email, resp.Phone)
	}
	if resp.Login != "Test2" || resp.Name != "Bob" {
		t.Fatalf("reduced shape should still carry login/name, got %+v", resp)
	}
	if resp.FriendStatus != profilesv1.FriendStatus_NONE {
		t.Fatalf("want NONE, got %v", resp.FriendStatus)
	}
}

func TestGetUserInfo_Friend_FullShape(t *testing.T) {
	s, store, _ := fixture()
	store.friendships[[2]string{"u1", "u2"}] = true
	store.friendships[[2]string{"u2", "u1"}] = true
	resp, err := s.GetUserInfo(ctxWithToken("tok1"), &profilesv1.GetUserInfoRequest{UserIdTarget: "u2"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !resp.HasPii || resp.Email != "b@x.io" || resp.Phone != "+200" {
		t.Fatalf("friend must get full shape with PII, got %+v", resp)
	}
	if resp.FriendStatus != profilesv1.FriendStatus_FRIENDS {
		t.Fatalf("want FRIENDS, got %v", resp.FriendStatus)
	}
}

func TestGetUserInfo_Self_FullShape(t *testing.T) {
	s, _, _ := fixture()
	resp, err := s.GetUserInfo(ctxWithToken("tok1"), &profilesv1.GetUserInfoRequest{UserIdTarget: "u1"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !resp.HasPii || resp.Email != "a@x.io" {
		t.Fatalf("self must see own PII, got %+v", resp)
	}
}

func TestGetUserInfo_NeverReturnsPresence(t *testing.T) {
	s, store, _ := fixture()
	store.friendships[[2]string{"u1", "u2"}] = true
	store.friendships[[2]string{"u2", "u1"}] = true
	resp, _ := s.GetUserInfo(ctxWithToken("tok1"), &profilesv1.GetUserInfoRequest{UserIdTarget: "u2"})
	if resp.OnWalk || resp.CurrentObjectId != "" {
		t.Fatal("Profiles must not report presence — Map owns it")
	}
}

// --- friend request rules ---------------------------------------------------

func TestSendFriendRequest_Happy(t *testing.T) {
	s, _, _ := fixture()
	resp, err := s.SendFriendRequest(ctxWithToken("tok1"), &profilesv1.SendFriendRequestRequest{UserIdTarget: "u2"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != codeOK || resp.FriendRequestId == "" {
		t.Fatalf("want request id, got %+v", resp)
	}
}

func TestSendFriendRequest_RejectedWhenAlreadyFriends(t *testing.T) {
	s, store, _ := fixture()
	store.friendships[[2]string{"u1", "u2"}] = true
	resp, _ := s.SendFriendRequest(ctxWithToken("tok1"), &profilesv1.SendFriendRequestRequest{UserIdTarget: "u2"})
	if resp.Code != codeConflict {
		t.Fatalf("want conflict, got %d", resp.Code)
	}
}

func TestSendFriendRequest_RejectedWhenPending(t *testing.T) {
	s, store, _ := fixture()
	store.requests["r"] = &pg.FriendRequest{ID: "r", FromUserID: "u1", ToUserID: "u2", Status: "PENDING"}
	resp, _ := s.SendFriendRequest(ctxWithToken("tok1"), &profilesv1.SendFriendRequestRequest{UserIdTarget: "u2"})
	if resp.Code != codeConflict {
		t.Fatalf("want conflict, got %d", resp.Code)
	}
}

func TestSendFriendRequest_RejectedWhenBlocked(t *testing.T) {
	s, store, _ := fixture()
	store.blocks[[2]string{"u2", "u1"}] = true // u2 blocked u1
	resp, _ := s.SendFriendRequest(ctxWithToken("tok1"), &profilesv1.SendFriendRequestRequest{UserIdTarget: "u2"})
	if resp.Code != codeForbidden {
		t.Fatalf("want forbidden, got %d", resp.Code)
	}
}

// --- accept flow refreshes both caches (Map contract) -----------------------

func TestSendFriendResponse_AcceptCreatesFriendshipAndRefreshesCaches(t *testing.T) {
	s, store, cache := fixture()
	store.requests["r"] = &pg.FriendRequest{ID: "r", FromUserID: "u1", ToUserID: "u2", Status: "PENDING"}
	// u2 (addressee) accepts.
	resp, err := s.SendFriendResponse(ctxWithToken("tok2"), &profilesv1.SendFriendResponseRequest{FriendRequestId: "r", Resolution: true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != codeOK {
		t.Fatalf("want ok, got %d %q", resp.Code, resp.Message)
	}
	if !store.friendships[[2]string{"u1", "u2"}] || !store.friendships[[2]string{"u2", "u1"}] {
		t.Fatal("friendship must be created both directions")
	}
	// Both friends:{uid} caches must be refreshed for Map.
	if got := cache.friends["u1"]; len(got) != 1 || got[0] != "u2" {
		t.Fatalf("friends:u1 not refreshed: %v", got)
	}
	if got := cache.friends["u2"]; len(got) != 1 || got[0] != "u1" {
		t.Fatalf("friends:u2 not refreshed: %v", got)
	}
}

func TestSendFriendResponse_OnlyAddresseeMayRespond(t *testing.T) {
	s, store, _ := fixture()
	store.requests["r"] = &pg.FriendRequest{ID: "r", FromUserID: "u1", ToUserID: "u2", Status: "PENDING"}
	// u1 (the sender) tries to accept their own request.
	resp, _ := s.SendFriendResponse(ctxWithToken("tok1"), &profilesv1.SendFriendResponseRequest{FriendRequestId: "r", Resolution: true})
	if resp.Code != codeForbidden {
		t.Fatalf("want forbidden, got %d", resp.Code)
	}
}

func TestSendFriendResponse_Decline(t *testing.T) {
	s, store, cache := fixture()
	store.requests["r"] = &pg.FriendRequest{ID: "r", FromUserID: "u1", ToUserID: "u2", Status: "PENDING"}
	resp, _ := s.SendFriendResponse(ctxWithToken("tok2"), &profilesv1.SendFriendResponseRequest{FriendRequestId: "r", Resolution: false})
	if resp.Code != codeOK {
		t.Fatalf("want ok, got %d", resp.Code)
	}
	if store.friendships[[2]string{"u1", "u2"}] {
		t.Fatal("decline must not create friendship")
	}
	if len(cache.friends) != 0 {
		t.Fatal("decline must not touch caches")
	}
}

// --- block cascade ----------------------------------------------------------

func TestBlockUser_RemovesFriendshipAndRefreshesCache(t *testing.T) {
	s, store, cache := fixture()
	store.friendships[[2]string{"u1", "u2"}] = true
	store.friendships[[2]string{"u2", "u1"}] = true
	resp, err := s.BlockUser(ctxWithToken("tok1"), &profilesv1.TargetRequest{UserIdTarget: "u2"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != codeOK {
		t.Fatalf("want ok, got %d", resp.Code)
	}
	if store.friendships[[2]string{"u1", "u2"}] || store.friendships[[2]string{"u2", "u1"}] {
		t.Fatal("block must remove friendship both directions")
	}
	// u1's cache should now be empty; refresh must have run for both.
	if len(cache.friends["u1"]) != 0 {
		t.Fatalf("friends:u1 should be empty after block, got %v", cache.friends["u1"])
	}
	if _, ok := cache.friends["u2"]; !ok {
		t.Fatal("friends:u2 must be refreshed after block")
	}
	// And future requests are blocked.
	blocked, _ := store.IsBlockedEitherWay(context.Background(), "u1", "u2")
	if !blocked {
		t.Fatal("block record missing")
	}
}

func TestUnblockUser(t *testing.T) {
	s, store, _ := fixture()
	store.blocks[[2]string{"u1", "u2"}] = true
	resp, _ := s.UnblockUser(ctxWithToken("tok1"), &profilesv1.TargetRequest{UserIdTarget: "u2"})
	if resp.Code != codeOK {
		t.Fatalf("want ok, got %d", resp.Code)
	}
	if store.blocks[[2]string{"u1", "u2"}] {
		t.Fatal("unblock must remove the block")
	}
}

// --- ListFriends ------------------------------------------------------------

func TestListFriends_ShapesIncomingOutgoing(t *testing.T) {
	s, store, _ := fixture()
	store.profiles["u3"] = &pg.Profile{UserID: "u3", Login: "Test3"}
	store.friendships[[2]string{"u1", "u2"}] = true
	store.requests["in"] = &pg.FriendRequest{ID: "in", FromUserID: "u3", ToUserID: "u1", Status: "PENDING"}
	store.requests["out"] = &pg.FriendRequest{ID: "out", FromUserID: "u1", ToUserID: "u3", Status: "PENDING"}

	resp, err := s.ListFriends(ctxWithToken("tok1"), &profilesv1.ListFriendsRequest{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(resp.Friends) != 1 || resp.Friends[0].UserId != "u2" {
		t.Fatalf("friends wrong: %+v", resp.Friends)
	}
	if len(resp.IncomingRequests) != 1 || resp.IncomingRequests[0].FromLogin != "Test3" {
		t.Fatalf("incoming wrong: %+v", resp.IncomingRequests)
	}
	if len(resp.OutgoingRequests) != 1 || resp.OutgoingRequests[0].ToUserId != "u3" {
		t.Fatalf("outgoing wrong: %+v", resp.OutgoingRequests)
	}
	// to_login lets the UI show a nickname instead of a UUID.
	if resp.OutgoingRequests[0].ToLogin != "Test3" {
		t.Fatalf("outgoing to_login = %q, want Test3", resp.OutgoingRequests[0].ToLogin)
	}
}

func TestListFriends_OutgoingToProfilelessTargetTolerated(t *testing.T) {
	s, store, _ := fixture()
	// Outgoing request to a user id with NO profiles row (to_user_id has no FK).
	// The LEFT JOIN must tolerate it: empty to_login, and the whole call still OK.
	store.requests["out"] = &pg.FriendRequest{ID: "out", FromUserID: "u1", ToUserID: "ghost", Status: "PENDING"}

	resp, err := s.ListFriends(ctxWithToken("tok1"), &profilesv1.ListFriendsRequest{})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != codeOK {
		t.Fatalf("code = %d, want OK (a dangling target must not fail ListFriends)", resp.Code)
	}
	if len(resp.OutgoingRequests) != 1 || resp.OutgoingRequests[0].ToUserId != "ghost" || resp.OutgoingRequests[0].ToLogin != "" {
		t.Fatalf("outgoing dangling wrong: %+v", resp.OutgoingRequests)
	}
}

// --- CreateProfile idempotency (Auth handoff) -------------------------------

func TestCreateProfile_Idempotent(t *testing.T) {
	s, store, _ := fixture()
	req := &profilesv1.CreateProfileRequest{UserId: "u9", Login: "New", Email: "n@x.io"}
	r1, _ := s.CreateProfile(context.Background(), req)
	r2, _ := s.CreateProfile(context.Background(), req) // retry
	if r1.Code != codeOK || r2.Code != codeOK {
		t.Fatalf("both calls must succeed: %d %d", r1.Code, r2.Code)
	}
	if _, ok := store.profiles["u9"]; !ok {
		t.Fatal("profile not created")
	}
	if store.createCalls != 2 {
		t.Fatalf("expected 2 store calls, got %d", store.createCalls)
	}
	// Second call must not overwrite (still one logical profile).
	if store.profiles["u9"].Login != "New" {
		t.Fatal("idempotent create must not clobber existing row")
	}
}

func TestCreateProfile_ValidatesRequired(t *testing.T) {
	s, _, _ := fixture()
	resp, _ := s.CreateProfile(context.Background(), &profilesv1.CreateProfileRequest{UserId: "", Login: "x", Email: "y"})
	if resp.Code != codeBadRequest {
		t.Fatalf("want bad request, got %d", resp.Code)
	}
}

// --- RemoveFriend -----------------------------------------------------------

func TestRemoveFriend_RefreshesCaches(t *testing.T) {
	s, store, cache := fixture()
	store.friendships[[2]string{"u1", "u2"}] = true
	store.friendships[[2]string{"u2", "u1"}] = true
	resp, _ := s.RemoveFriend(ctxWithToken("tok1"), &profilesv1.TargetRequest{UserIdTarget: "u2"})
	if resp.Code != codeOK {
		t.Fatalf("want ok, got %d", resp.Code)
	}
	if store.friendships[[2]string{"u1", "u2"}] {
		t.Fatal("friendship not removed")
	}
	if _, ok := cache.friends["u1"]; !ok {
		t.Fatal("cache must be refreshed for u1")
	}
	if _, ok := cache.friends["u2"]; !ok {
		t.Fatal("cache must be refreshed for u2")
	}
}

// --- FindUserByLogin (find-friend-by-login discovery) -----------------------

func TestFindUserByLogin_NoToken_Unauthorized(t *testing.T) {
	s, _, _ := fixture()
	resp, err := s.FindUserByLogin(context.Background(), &profilesv1.FindUserByLoginRequest{Login: "Test2"})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if resp.Code != codeUnauthorized {
		t.Fatalf("want %d, got %d", codeUnauthorized, resp.Code)
	}
}

func TestFindUserByLogin_Found_ReducedShapeNoPII(t *testing.T) {
	s, store, _ := fixture()
	// Give u2 a pet so we prove non-PII fields still come through.
	store.profiles["u2"].Pets = []pg.Pet{{Breed: "Poodle", Name: "Bruno", Sex: "M", IsCastrated: true, Age: 3}}
	// u1 searches for u2 (a stranger).
	resp, err := s.FindUserByLogin(ctxWithToken("tok1"), &profilesv1.FindUserByLoginRequest{Login: "Test2"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != codeOK {
		t.Fatalf("want ok, got code=%d msg=%q", resp.Code, resp.Message)
	}
	if resp.HasPii {
		t.Fatal("discovery lookup must return reduced shape (has_pii=false)")
	}
	if resp.Email != "" || resp.Phone != "" {
		t.Fatalf("PII leaked from find-by-login: email=%q phone=%q", resp.Email, resp.Phone)
	}
	if resp.CurrentObjectId != "" || resp.OnWalk {
		t.Fatal("Profiles must not report presence")
	}
	// Non-PII discovery fields must be present so the frontend can show the card.
	if resp.UserId != "u2" || resp.Login != "Test2" || resp.Name != "Bob" {
		t.Fatalf("reduced shape should carry user_id/login/name, got %+v", resp)
	}
	if len(resp.Pets) != 1 || resp.Pets[0].Name != "Bruno" {
		t.Fatalf("pets should be included in reduced shape, got %+v", resp.Pets)
	}
	if resp.FriendStatus != profilesv1.FriendStatus_NONE {
		t.Fatalf("want NONE, got %v", resp.FriendStatus)
	}
}

func TestFindUserByLogin_CaseInsensitive(t *testing.T) {
	s, _, _ := fixture()
	// Stored login is "Test2"; searching lower-case must still find it (citext).
	resp, err := s.FindUserByLogin(ctxWithToken("tok1"), &profilesv1.FindUserByLoginRequest{Login: "test2"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.Code != codeOK || resp.UserId != "u2" {
		t.Fatalf("case-insensitive lookup failed: code=%d id=%q", resp.Code, resp.UserId)
	}
}

func TestFindUserByLogin_NotFound_Envelope(t *testing.T) {
	s, _, _ := fixture()
	resp, err := s.FindUserByLogin(ctxWithToken("tok1"), &profilesv1.FindUserByLoginRequest{Login: "ghost"})
	if err != nil {
		t.Fatalf("not-found must be an envelope, not a Go error: %v", err)
	}
	if resp.Code != codeNotFound {
		t.Fatalf("want %d, got %d", codeNotFound, resp.Code)
	}
	if resp.Message == "" {
		t.Fatal("not-found envelope must carry a message")
	}
	if resp.UserId != "" {
		t.Fatalf("not-found must not leak a user, got %q", resp.UserId)
	}
}

// Even when the looked-up user is already a friend, discovery stays reduced
// (no PII) — but friend_status must reflect the real relationship so the
// frontend renders the correct button.
func TestFindUserByLogin_FriendStillReducedButStatusFriends(t *testing.T) {
	s, store, _ := fixture()
	store.friendships[[2]string{"u1", "u2"}] = true
	store.friendships[[2]string{"u2", "u1"}] = true
	resp, err := s.FindUserByLogin(ctxWithToken("tok1"), &profilesv1.FindUserByLoginRequest{Login: "Test2"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.HasPii || resp.Email != "" || resp.Phone != "" {
		t.Fatalf("find-by-login must stay reduced even for a friend, got %+v", resp)
	}
	if resp.FriendStatus != profilesv1.FriendStatus_FRIENDS {
		t.Fatalf("want FRIENDS status, got %v", resp.FriendStatus)
	}
}

func TestFindUserByLogin_BlankLogin_BadRequest(t *testing.T) {
	s, _, _ := fixture()
	resp, _ := s.FindUserByLogin(ctxWithToken("tok1"), &profilesv1.FindUserByLoginRequest{Login: "   "})
	if resp.Code != codeBadRequest {
		t.Fatalf("want bad request, got %d", resp.Code)
	}
}
