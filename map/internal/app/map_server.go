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
func (s *Server) view(ctx context.Context, o postgres.Object, caller string) (*mapv1.MapObject, error) {
	count, err := s.presence.VisitorCount(ctx, o.ID)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read presence")
	}
	friends, err := s.presence.FriendIDsHere(ctx, o.ID, caller)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to read presence")
	}
	return &mapv1.MapObject{
		Id:            o.ID,
		ObjectType:    objectTypeToProto(o.ObjectType),
		Longitude:     o.Longitude,
		Latitude:      o.Latitude,
		VisitorCount:  uint32(count),
		FriendIdsHere: friends,
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

	out := make([]*mapv1.MapObject, 0, len(objs))
	for _, o := range objs {
		v, err := s.view(ctx, o, caller)
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

	v, err := s.view(ctx, o, caller)
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

	v, err := s.view(ctx, o, caller)
	if err != nil {
		return nil, err
	}
	return &mapv1.MapObjectResponse{Code: 0, Message: "ok", Object: v}, nil
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
