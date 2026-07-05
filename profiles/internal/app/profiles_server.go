package app

import (
	"context"
	"errors"
	"strings"

	pg "profiles/internal/domain/postgres"
	profilesv1 "profiles/pkg/api/profiles/v1"

	"google.golang.org/grpc/metadata"
)

// authHeader is the metadata key carrying the opaque session token. The REST
// gateway forwards the `auth_token` HTTP header as grpc metadata prefixed with
// "grpcgateway-".
const (
	authHeader        = "auth_token"
	gatewayAuthHeader = "grpcgateway-auth_token"
)

// Error codes returned in the {code, message} envelope. 0 == success.
const (
	codeOK           = 0
	codeUnauthorized = 401
	codeNotFound     = 404
	codeConflict     = 409
	codeBadRequest   = 400
	codeForbidden    = 403
	codeInternal     = 500
)

// profileStore is the narrow slice of postgres the server depends on, so it can
// be faked in unit tests.
type profileStore interface {
	CreateProfile(ctx context.Context, userID, login, email string) error
	GetProfile(ctx context.Context, userID string) (*pg.Profile, error)
	GetProfileByLogin(ctx context.Context, login string) (*pg.Profile, error)
	LoginFor(ctx context.Context, userID string) (string, error)
	UpdateProfile(ctx context.Context, userID, name, surname, phone string, pets []pg.Pet) error

	AreFriends(ctx context.Context, a, b string) (bool, error)
	IsBlockedEitherWay(ctx context.Context, a, b string) (bool, error)
	HasBlocked(ctx context.Context, blocker, blocked string) (bool, error)
	PendingBetween(ctx context.Context, a, b string) (*pg.FriendRequest, error)
	CreateFriendRequest(ctx context.Context, from, to string) (string, error)
	GetPendingRequest(ctx context.Context, id string) (*pg.FriendRequest, error)
	AcceptFriendRequest(ctx context.Context, id, a, b string) error
	DeclineFriendRequest(ctx context.Context, id string) error
	RemoveFriendship(ctx context.Context, a, b string) error
	Block(ctx context.Context, blocker, blocked string) error
	Unblock(ctx context.Context, blocker, blocked string) error
	FriendIDs(ctx context.Context, userID string) ([]string, error)
	Friends(ctx context.Context, userID string) ([]pg.FriendRef, error)
	IncomingPending(ctx context.Context, userID string) ([]pg.FriendRequest, map[string]string, error)
	OutgoingPending(ctx context.Context, userID string) ([]pg.FriendRequest, map[string]string, error)
}

// friendCache is the friends:{uid} Valkey cache + session resolution.
type friendCache interface {
	ResolveSession(ctx context.Context, token string) (string, error)
	SetFriends(ctx context.Context, userID string, friendIDs []string) error
}

// Server implements the generated ProfilesServiceServer. It derives the acting
// user from the session token (never the body) and enforces the friends-only
// PII rule and the friend-graph invariants.
type Server struct {
	profilesv1.UnimplementedProfilesServiceServer
	store profileStore
	cache friendCache
}

func NewServer(store profileStore, cache friendCache) *Server {
	return &Server{store: store, cache: cache}
}

// actingUser resolves the acting user id from the session token in metadata.
// Returns "" (caller maps to unauthorized) when there is no valid session.
func (s *Server) actingUser(ctx context.Context) (string, error) {
	token := tokenFromContext(ctx)
	if token == "" {
		return "", nil
	}
	return s.cache.ResolveSession(ctx, token)
}

func tokenFromContext(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, key := range []string{authHeader, gatewayAuthHeader} {
		if v := md.Get(key); len(v) > 0 && v[0] != "" {
			return v[0]
		}
	}
	return ""
}

// --- edge RPCs ---

// GetUserInfo returns the full profile to self/friends and the reduced (no PII)
// shape to non-friends. friend_status is computed for the caller.
func (s *Server) GetUserInfo(ctx context.Context, req *profilesv1.GetUserInfoRequest) (*profilesv1.GetUserInfoResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.GetUserInfoResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}
	target := strings.TrimSpace(req.GetUserIdTarget())
	if target == "" {
		return &profilesv1.GetUserInfoResponse{Code: codeBadRequest, Message: "user_id_target required"}, nil
	}

	profile, err := s.store.GetProfile(ctx, target)
	if errors.Is(err, pg.ErrNotFound) {
		return &profilesv1.GetUserInfoResponse{Code: codeNotFound, Message: "user not found"}, nil
	}
	if err != nil {
		return &profilesv1.GetUserInfoResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	status, err := s.friendStatus(ctx, caller, target)
	if err != nil {
		return &profilesv1.GetUserInfoResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	// Full shape is returned to self or friends only. PII (email/phone) and
	// current_object_id are stripped for everyone else.
	resp := reducedUserInfo(profile, status)
	if caller == target || status == profilesv1.FriendStatus_FRIENDS {
		resp.HasPii = true
		resp.Email = profile.Email
		resp.Phone = profile.Phone
		// current_object_id would be filled by Map; Profiles leaves it empty.
	}
	return resp, nil
}

// FindUserByLogin is the find-friend-by-login discovery lookup. It resolves the
// login (case-insensitive; login is citext) and ALWAYS returns the reduced
// (no-PII) shape — even when the looked-up user is already a friend — so this
// search endpoint is never a PII surface. friend_status is still computed for
// the caller so the frontend can render the right button. A missing login is a
// {code, message} not-found envelope, not a Go/gRPC error.
func (s *Server) FindUserByLogin(ctx context.Context, req *profilesv1.FindUserByLoginRequest) (*profilesv1.GetUserInfoResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.GetUserInfoResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}
	login := strings.TrimSpace(req.GetLogin())
	if login == "" {
		return &profilesv1.GetUserInfoResponse{Code: codeBadRequest, Message: "login required"}, nil
	}

	profile, err := s.store.GetProfileByLogin(ctx, login)
	if errors.Is(err, pg.ErrNotFound) {
		return &profilesv1.GetUserInfoResponse{Code: codeNotFound, Message: "user not found"}, nil
	}
	if err != nil {
		return &profilesv1.GetUserInfoResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	status, err := s.friendStatus(ctx, caller, profile.UserID)
	if err != nil {
		return &profilesv1.GetUserInfoResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	// Discovery never exposes PII — even for a friend. Frontend uses the full
	// GetUserInfo path (friends panel) when it needs email/phone.
	return reducedUserInfo(profile, status), nil
}

// EditUser updates the acting user's own profile. No user_id in the body; login
// and email are not editable here.
func (s *Server) EditUser(ctx context.Context, req *profilesv1.EditUserRequest) (*profilesv1.GetUserInfoResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.GetUserInfoResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}
	for _, p := range req.GetPets() {
		if p.GetSex() != "M" && p.GetSex() != "F" {
			return &profilesv1.GetUserInfoResponse{Code: codeBadRequest, Message: "pet sex must be M or F"}, nil
		}
		if p.GetAge() < 0 {
			return &profilesv1.GetUserInfoResponse{Code: codeBadRequest, Message: "pet age must be >= 0"}, nil
		}
	}

	err = s.store.UpdateProfile(ctx, caller, req.GetName(), req.GetSurname(), req.GetPhone(), fromProtoPets(req.GetPets()))
	if errors.Is(err, pg.ErrNotFound) {
		return &profilesv1.GetUserInfoResponse{Code: codeNotFound, Message: "profile not found"}, nil
	}
	if err != nil {
		return &profilesv1.GetUserInfoResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	profile, err := s.store.GetProfile(ctx, caller)
	if err != nil {
		return &profilesv1.GetUserInfoResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	return &profilesv1.GetUserInfoResponse{
		Code:         codeOK,
		UserId:       profile.UserID,
		Login:        profile.Login,
		Name:         profile.Name,
		Surname:      profile.Surname,
		Email:        profile.Email,
		Phone:        profile.Phone,
		Pets:         toProtoPets(profile.Pets),
		FriendStatus: profilesv1.FriendStatus_NONE,
		HasPii:       true,
	}, nil
}

// SendFriendRequest is rejected if the two are blocked (either way), already
// friends, or a pending request already exists between them.
func (s *Server) SendFriendRequest(ctx context.Context, req *profilesv1.SendFriendRequestRequest) (*profilesv1.SendFriendRequestResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.SendFriendRequestResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}
	target := strings.TrimSpace(req.GetUserIdTarget())
	if target == "" || target == caller {
		return &profilesv1.SendFriendRequestResponse{Code: codeBadRequest, Message: "invalid target"}, nil
	}

	if blocked, err := s.store.IsBlockedEitherWay(ctx, caller, target); err != nil {
		return &profilesv1.SendFriendRequestResponse{Code: codeInternal, Message: "internal error"}, nil
	} else if blocked {
		return &profilesv1.SendFriendRequestResponse{Code: codeForbidden, Message: "blocked"}, nil
	}
	if friends, err := s.store.AreFriends(ctx, caller, target); err != nil {
		return &profilesv1.SendFriendRequestResponse{Code: codeInternal, Message: "internal error"}, nil
	} else if friends {
		return &profilesv1.SendFriendRequestResponse{Code: codeConflict, Message: "already friends"}, nil
	}
	if _, err := s.store.PendingBetween(ctx, caller, target); err == nil {
		return &profilesv1.SendFriendRequestResponse{Code: codeConflict, Message: "request already pending"}, nil
	} else if !errors.Is(err, pg.ErrNotFound) {
		return &profilesv1.SendFriendRequestResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	id, err := s.store.CreateFriendRequest(ctx, caller, target)
	if err != nil {
		return &profilesv1.SendFriendRequestResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	return &profilesv1.SendFriendRequestResponse{Code: codeOK, FriendRequestId: id}, nil
}

// SendFriendResponse accepts or declines a pending request. Only the addressee
// may respond. On accept it creates the friendship both directions and refreshes
// both users' friends:{uid} caches.
func (s *Server) SendFriendResponse(ctx context.Context, req *profilesv1.SendFriendResponseRequest) (*profilesv1.StatusResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.StatusResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}

	fr, err := s.store.GetPendingRequest(ctx, strings.TrimSpace(req.GetFriendRequestId()))
	if errors.Is(err, pg.ErrNotFound) {
		return &profilesv1.StatusResponse{Code: codeNotFound, Message: "request not found"}, nil
	}
	if err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	// Only the addressee may resolve the request.
	if fr.ToUserID != caller {
		return &profilesv1.StatusResponse{Code: codeForbidden, Message: "not your request"}, nil
	}

	if !req.GetResolution() {
		if err := s.store.DeclineFriendRequest(ctx, fr.ID); err != nil {
			return &profilesv1.StatusResponse{Code: codeInternal, Message: "internal error"}, nil
		}
		return &profilesv1.StatusResponse{Code: codeOK, Message: "declined"}, nil
	}

	if err := s.store.AcceptFriendRequest(ctx, fr.ID, fr.FromUserID, fr.ToUserID); err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	if err := s.refreshFriendCaches(ctx, fr.FromUserID, fr.ToUserID); err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "cache refresh failed"}, nil
	}
	return &profilesv1.StatusResponse{Code: codeOK, Message: "accepted"}, nil
}

// ListFriends returns the caller's friends plus incoming/outgoing pending
// requests. No body id — the caller is the token owner.
func (s *Server) ListFriends(ctx context.Context, _ *profilesv1.ListFriendsRequest) (*profilesv1.ListFriendsResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.ListFriendsResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}

	friends, err := s.store.Friends(ctx, caller)
	if err != nil {
		return &profilesv1.ListFriendsResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	incoming, logins, err := s.store.IncomingPending(ctx, caller)
	if err != nil {
		return &profilesv1.ListFriendsResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	outgoing, outLogins, err := s.store.OutgoingPending(ctx, caller)
	if err != nil {
		return &profilesv1.ListFriendsResponse{Code: codeInternal, Message: "internal error"}, nil
	}

	resp := &profilesv1.ListFriendsResponse{Code: codeOK}
	for _, f := range friends {
		// on_walk / current_object_id are owned by Map; Profiles reports defaults.
		resp.Friends = append(resp.Friends, &profilesv1.Friend{UserId: f.UserID, Login: f.Login})
	}
	for _, r := range incoming {
		resp.IncomingRequests = append(resp.IncomingRequests, &profilesv1.IncomingRequest{
			FromUserId: r.FromUserID, FromLogin: logins[r.FromUserID], FriendRequestId: r.ID,
		})
	}
	for _, r := range outgoing {
		// to_login (LEFT JOIN, empty for a dangling request) → nickname not UUID.
		resp.OutgoingRequests = append(resp.OutgoingRequests, &profilesv1.OutgoingRequest{
			ToUserId: r.ToUserID, ToLogin: outLogins[r.ToUserID], FriendRequestId: r.ID,
		})
	}
	return resp, nil
}

// RemoveFriend unfriends target and refreshes both caches.
func (s *Server) RemoveFriend(ctx context.Context, req *profilesv1.TargetRequest) (*profilesv1.StatusResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.StatusResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}
	target := strings.TrimSpace(req.GetUserIdTarget())
	if target == "" || target == caller {
		return &profilesv1.StatusResponse{Code: codeBadRequest, Message: "invalid target"}, nil
	}
	if err := s.store.RemoveFriendship(ctx, caller, target); err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	if err := s.refreshFriendCaches(ctx, caller, target); err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "cache refresh failed"}, nil
	}
	return &profilesv1.StatusResponse{Code: codeOK, Message: "removed"}, nil
}

// BlockUser blocks target: removes any friendship + pending requests and
// refreshes both caches so Map immediately stops leaking presence.
func (s *Server) BlockUser(ctx context.Context, req *profilesv1.TargetRequest) (*profilesv1.StatusResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.StatusResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}
	target := strings.TrimSpace(req.GetUserIdTarget())
	if target == "" || target == caller {
		return &profilesv1.StatusResponse{Code: codeBadRequest, Message: "invalid target"}, nil
	}
	if err := s.store.Block(ctx, caller, target); err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	if err := s.refreshFriendCaches(ctx, caller, target); err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "cache refresh failed"}, nil
	}
	return &profilesv1.StatusResponse{Code: codeOK, Message: "blocked"}, nil
}

// UnblockUser removes the caller→target block (does not restore friendship).
func (s *Server) UnblockUser(ctx context.Context, req *profilesv1.TargetRequest) (*profilesv1.StatusResponse, error) {
	caller, err := s.actingUser(ctx)
	if err != nil {
		return nil, err
	}
	if caller == "" {
		return &profilesv1.StatusResponse{Code: codeUnauthorized, Message: "unauthorized"}, nil
	}
	target := strings.TrimSpace(req.GetUserIdTarget())
	if target == "" || target == caller {
		return &profilesv1.StatusResponse{Code: codeBadRequest, Message: "invalid target"}, nil
	}
	if err := s.store.Unblock(ctx, caller, target); err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	return &profilesv1.StatusResponse{Code: codeOK, Message: "unblocked"}, nil
}

// CreateProfile is the internal Auth→Profiles handoff. Idempotent; not exposed
// at the HTTP edge (no google.api.http annotation on the proto).
func (s *Server) CreateProfile(ctx context.Context, req *profilesv1.CreateProfileRequest) (*profilesv1.StatusResponse, error) {
	userID := strings.TrimSpace(req.GetUserId())
	login := strings.TrimSpace(req.GetLogin())
	email := strings.TrimSpace(req.GetEmail())
	if userID == "" || login == "" || email == "" {
		return &profilesv1.StatusResponse{Code: codeBadRequest, Message: "user_id, login, email required"}, nil
	}
	if err := s.store.CreateProfile(ctx, userID, login, email); err != nil {
		return &profilesv1.StatusResponse{Code: codeInternal, Message: "internal error"}, nil
	}
	return &profilesv1.StatusResponse{Code: codeOK, Message: "ok"}, nil
}

// --- helpers ---

// friendStatus computes the caller's relationship to target (block dominates).
func (s *Server) friendStatus(ctx context.Context, caller, target string) (profilesv1.FriendStatus, error) {
	if caller == target {
		return profilesv1.FriendStatus_NONE, nil
	}
	if blocked, err := s.store.IsBlockedEitherWay(ctx, caller, target); err != nil {
		return 0, err
	} else if blocked {
		return profilesv1.FriendStatus_BLOCKED, nil
	}
	if friends, err := s.store.AreFriends(ctx, caller, target); err != nil {
		return 0, err
	} else if friends {
		return profilesv1.FriendStatus_FRIENDS, nil
	}
	if fr, err := s.store.PendingBetween(ctx, caller, target); err == nil {
		if fr.FromUserID == caller {
			return profilesv1.FriendStatus_PENDING_OUT, nil
		}
		return profilesv1.FriendStatus_PENDING_IN, nil
	} else if !errors.Is(err, pg.ErrNotFound) {
		return 0, err
	}
	return profilesv1.FriendStatus_NONE, nil
}

// refreshFriendCaches rebuilds friends:{uid} for each user from Postgres — the
// source of truth Map reads for presence privacy (SINTER).
func (s *Server) refreshFriendCaches(ctx context.Context, users ...string) error {
	for _, u := range users {
		ids, err := s.store.FriendIDs(ctx, u)
		if err != nil {
			return err
		}
		if err := s.cache.SetFriends(ctx, u, ids); err != nil {
			return err
		}
	}
	return nil
}

// reducedUserInfo builds the no-PII UserInfo shape: user_id, login, name,
// surname, pets, friend_status — but never email/phone/current_object_id, and
// has_pii=false. GetUserInfo layers PII on top of this for self/friends;
// FindUserByLogin returns it verbatim. on_walk/current_object_id stay at their
// zero values because Map, not Profiles, owns presence.
func reducedUserInfo(p *pg.Profile, status profilesv1.FriendStatus) *profilesv1.GetUserInfoResponse {
	return &profilesv1.GetUserInfoResponse{
		Code:         codeOK,
		UserId:       p.UserID,
		Login:        p.Login,
		Name:         p.Name,
		Surname:      p.Surname,
		Pets:         toProtoPets(p.Pets),
		OnWalk:       false,
		FriendStatus: status,
		HasPii:       false,
	}
}

func toProtoPets(pets []pg.Pet) []*profilesv1.Pet {
	out := make([]*profilesv1.Pet, 0, len(pets))
	for _, p := range pets {
		out = append(out, &profilesv1.Pet{
			Breed: p.Breed, Name: p.Name, Sex: p.Sex, IsCastrated: p.IsCastrated, Age: p.Age,
		})
	}
	return out
}

func fromProtoPets(pets []*profilesv1.Pet) []pg.Pet {
	out := make([]pg.Pet, 0, len(pets))
	for _, p := range pets {
		out = append(out, pg.Pet{
			Breed: p.GetBreed(), Name: p.GetName(), Sex: p.GetSex(),
			IsCastrated: p.GetIsCastrated(), Age: p.GetAge(),
		})
	}
	return out
}
