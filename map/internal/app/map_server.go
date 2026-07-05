package app

import (
	"context"
	"errors"
	"time"

	"map-service/internal/domain/postgres"
	mapv1 "map-service/pkg/api/map-service/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// authTokenHeader is the metadata key carrying the opaque session token. The
// grpc-gateway lowercases and forwards the incoming `auth_token` HTTP header
// under this key.
const authTokenHeader = "auth_token"

// ObjectStore is the persistence surface the server depends on (satisfied by
// domain/postgres.Store). Depending on an interface lets us unit-test the server
// with a fake and no database.
type ObjectStore interface {
	ObjectsWithin(ctx context.Context, lon, lat float64, radiusMeters int) ([]postgres.Object, error)
	ObjectByID(ctx context.Context, id string) (postgres.Object, error)
}

// PresenceStore is the ephemeral-presence surface the server depends on
// (satisfied by domain/valkey.Store).
type PresenceStore interface {
	MarkVisiting(ctx context.Context, user, object string, ttl time.Duration) error
	MarkNotVisiting(ctx context.Context, user, object string) error
	VisitorCount(ctx context.Context, object string) (int, error)
	FriendIDsHere(ctx context.Context, object, caller string) ([]string, error)
	// CurrentObject returns the object the caller currently holds presence in, or
	// "" if none — used to set viewer_visiting on the returned objects.
	CurrentObject(ctx context.Context, user string) (string, error)
	// Friends returns the caller's cached friend set (friends:{caller}). Read-only
	// — Profiles owns the set. Used by FriendsPresence to enumerate friends.
	Friends(ctx context.Context, caller string) ([]string, error)
}

// TokenResolver maps an opaque session token to the acting user id. Auth owns
// the session store; for MVP Map resolves the token against the shared Valkey
// session:{token} key via this interface. The acting user id ALWAYS comes from
// here — never from the request body.
type TokenResolver interface {
	UserIDForToken(ctx context.Context, token string) (string, error)
}

// Server implements the generated MapServiceServer. It holds no presence state
// of its own; it composes the object store, the presence store, and token
// resolution, and enforces the privacy model on every response.
type Server struct {
	mapv1.UnimplementedMapServiceServer

	objects  ObjectStore
	presence PresenceStore
	auth     TokenResolver

	radiusMeters int
	presenceTTL  time.Duration
}

// NewServer constructs the Map RPC implementation.
func NewServer(objects ObjectStore, presence PresenceStore, auth TokenResolver, radiusMeters int, presenceTTL time.Duration) *Server {
	if radiusMeters <= 0 {
		radiusMeters = 5000
	}
	if presenceTTL <= 0 {
		presenceTTL = 15 * time.Minute
	}
	return &Server{
		objects:      objects,
		presence:     presence,
		auth:         auth,
		radiusMeters: radiusMeters,
		presenceTTL:  presenceTTL,
	}
}

// callerID resolves the acting user id from the auth_token header. This is the
// single source of the acting identity; the request body is never consulted for
// it. Returns codes.Unauthenticated when the token is missing or invalid.
func (s *Server) callerID(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", status.Error(codes.Unauthenticated, "missing auth_token")
	}
	vals := md.Get(authTokenHeader)
	if len(vals) == 0 || vals[0] == "" {
		return "", status.Error(codes.Unauthenticated, "missing auth_token")
	}
	uid, err := s.auth.UserIDForToken(ctx, vals[0])
	if err != nil || uid == "" {
		return "", status.Error(codes.Unauthenticated, "invalid or expired session")
	}
	return uid, nil
}

// view builds the privacy-filtered client shape for one object: visitor_count for
// everyone, friend_ids_here computed for the caller. The raw visitor set never
// leaves this function.
func (s *Server) view(ctx context.Context, o postgres.Object, caller, callerObject string) (*mapv1.MapObject, error) {
	count, err := s.presence.VisitorCount(ctx, o.ID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read presence")
	}
	friends, err := s.presence.FriendIDsHere(ctx, o.ID, caller)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read presence")
	}
	return &mapv1.MapObject{
		Id:             o.ID,
		ObjectType:     objectTypeToProto(o.ObjectType),
		Longitude:      o.Longitude,
		Latitude:       o.Latitude,
		VisitorCount:   uint32(count),
		FriendIdsHere:  friends,
		ViewerVisiting: o.ID == callerObject && callerObject != "",
		Name:           o.Name,
	}, nil
}

// LoadMap returns objects within the configured radius (5km) of the caller's
// point, each with its privacy-filtered presence view.
func (s *Server) LoadMap(ctx context.Context, req *mapv1.LoadMapRequest) (*mapv1.LoadMapResponse, error) {
	caller, err := s.callerID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetLongitude() < -180 || req.GetLongitude() > 180 ||
		req.GetLatitude() < -90 || req.GetLatitude() > 90 {
		return nil, status.Error(codes.InvalidArgument, "longitude/latitude out of range")
	}

	objs, err := s.objects.ObjectsWithin(ctx, req.GetLongitude(), req.GetLatitude(), s.radiusMeters)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load map objects")
	}

	// One lookup of the caller's own presence covers every object's viewer_visiting.
	callerObject, _ := s.presence.CurrentObject(ctx, caller)

	out := make([]*mapv1.MapObject, 0, len(objs))
	for _, o := range objs {
		v, err := s.view(ctx, o, caller, callerObject)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return &mapv1.LoadMapResponse{Code: 0, Message: "ok", Objects: out}, nil
}

// GetMapObject returns a single object with the caller's presence view.
func (s *Server) GetMapObject(ctx context.Context, req *mapv1.GetMapObjectRequest) (*mapv1.MapObjectResponse, error) {
	caller, err := s.callerID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	o, err := s.objects.ObjectByID(ctx, req.GetId())
	if errors.Is(err, postgres.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "map object not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load map object")
	}

	callerObject, _ := s.presence.CurrentObject(ctx, caller)
	v, err := s.view(ctx, o, caller, callerObject)
	if err != nil {
		return nil, err
	}
	return &mapv1.MapObjectResponse{Code: 0, Message: "ok", Object: v}, nil
}

// ChangeMapObjectStatus marks the CALLER (token owner, never a body id) VISITING
// or NOT_VISITING the target object, then returns the updated view.
func (s *Server) ChangeMapObjectStatus(ctx context.Context, req *mapv1.ChangeMapObjectStatusRequest) (*mapv1.MapObjectResponse, error) {
	caller, err := s.callerID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	// The object must exist before we mutate presence against it.
	o, err := s.objects.ObjectByID(ctx, req.GetId())
	if errors.Is(err, postgres.ErrNotFound) {
		return nil, status.Error(codes.NotFound, "map object not found")
	}
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to load map object")
	}

	switch req.GetAction() {
	case mapv1.PresenceAction_VISITING:
		if err := s.presence.MarkVisiting(ctx, caller, o.ID, s.presenceTTL); err != nil {
			return nil, status.Error(codes.Internal, "failed to mark visiting")
		}
	case mapv1.PresenceAction_NOT_VISITING:
		if err := s.presence.MarkNotVisiting(ctx, caller, o.ID); err != nil {
			return nil, status.Error(codes.Internal, "failed to mark not visiting")
		}
	default:
		return nil, status.Error(codes.InvalidArgument, "action must be VISITING or NOT_VISITING")
	}

	// After the mutation the caller's presence reflects the new state (the object
	// just marked, or "" after NOT_VISITING), so viewer_visiting is accurate.
	callerObject, _ := s.presence.CurrentObject(ctx, caller)
	v, err := s.view(ctx, o, caller, callerObject)
	if err != nil {
		return nil, err
	}
	return &mapv1.MapObjectResponse{Code: 0, Message: "ok", Object: v}, nil
}

// FriendsPresence returns where the caller's friends currently on a walk are. For
// each friend in friends:{caller} that holds a live presence:{friend} key, it
// resolves the object row and emits {user_id, object_id, object_name, lat, lon}.
// Friends with no live presence — or whose object row has since disappeared — are
// silently skipped, so the list reflects only friends actually on a walk right now.
// The caller is the token owner (no body id); the friend set is Profiles-owned and
// read-only here.
func (s *Server) FriendsPresence(ctx context.Context, _ *mapv1.FriendsPresenceRequest) (*mapv1.FriendsPresenceResponse, error) {
	caller, err := s.callerID(ctx)
	if err != nil {
		return nil, err
	}

	friendIDs, err := s.presence.Friends(ctx, caller)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read friends")
	}

	// Cache object lookups so several friends at the same object hit the DB once.
	objCache := make(map[string]postgres.Object)
	out := make([]*mapv1.FriendPresence, 0, len(friendIDs))
	for _, friend := range friendIDs {
		objectID, err := s.presence.CurrentObject(ctx, friend)
		if err != nil {
			return nil, status.Error(codes.Internal, "failed to read presence")
		}
		if objectID == "" {
			continue // not on a walk right now
		}

		o, ok := objCache[objectID]
		if !ok {
			o, err = s.objects.ObjectByID(ctx, objectID)
			if errors.Is(err, postgres.ErrNotFound) {
				continue // object gone; skip rather than emit a dangling pin
			}
			if err != nil {
				return nil, status.Error(codes.Internal, "failed to load map object")
			}
			objCache[objectID] = o
		}

		out = append(out, &mapv1.FriendPresence{
			UserId:     friend,
			ObjectId:   o.ID,
			ObjectName: o.Name,
			Latitude:   o.Latitude,
			Longitude:  o.Longitude,
		})
	}

	return &mapv1.FriendsPresenceResponse{Code: 0, Message: "ok", Friends: out}, nil
}

func objectTypeToProto(t string) mapv1.ObjectType {
	switch t {
	case "PARK":
		return mapv1.ObjectType_PARK
	case "DOG_PARK":
		return mapv1.ObjectType_DOG_PARK
	case "DOG_BEACH":
		return mapv1.ObjectType_DOG_BEACH
	default:
		return mapv1.ObjectType_OBJECT_TYPE_UNSPECIFIED
	}
}
