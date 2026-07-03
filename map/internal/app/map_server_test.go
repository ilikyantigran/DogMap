package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"map-service/internal/domain/postgres"
	mapv1 "map-service/pkg/api/map-service/v1"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// --- fakes ---------------------------------------------------------------

type fakeObjects struct {
	within    []postgres.Object
	withinErr error
	byID      map[string]postgres.Object

	lastLon, lastLat float64
	lastRadius       int
}

func (f *fakeObjects) ObjectsWithin(_ context.Context, lon, lat float64, radius int) ([]postgres.Object, error) {
	f.lastLon, f.lastLat, f.lastRadius = lon, lat, radius
	return f.within, f.withinErr
}

func (f *fakeObjects) ObjectByID(_ context.Context, id string) (postgres.Object, error) {
	o, ok := f.byID[id]
	if !ok {
		return postgres.Object{}, postgres.ErrNotFound
	}
	return o, nil
}

// fakePresence records mutations so tests can assert the acting user id.
type fakePresence struct {
	counts  map[string]int      // object -> visitor_count
	friends map[string][]string // object -> friend ids for the caller

	markedVisiting    []markCall
	markedNotVisiting []markCall
}

type markCall struct {
	user, object string
	ttl          time.Duration
}

func (f *fakePresence) MarkVisiting(_ context.Context, user, object string, ttl time.Duration) error {
	f.markedVisiting = append(f.markedVisiting, markCall{user, object, ttl})
	if f.counts == nil {
		f.counts = map[string]int{}
	}
	f.counts[object]++
	return nil
}

func (f *fakePresence) MarkNotVisiting(_ context.Context, user, object string) error {
	f.markedNotVisiting = append(f.markedNotVisiting, markCall{user: user, object: object})
	if f.counts != nil && f.counts[object] > 0 {
		f.counts[object]--
	}
	return nil
}

func (f *fakePresence) VisitorCount(_ context.Context, object string) (int, error) {
	return f.counts[object], nil
}

func (f *fakePresence) FriendIDsHere(_ context.Context, object, _ string) ([]string, error) {
	return f.friends[object], nil
}

type fakeAuth struct {
	byToken map[string]string // token -> user id
}

func (f *fakeAuth) UserIDForToken(_ context.Context, token string) (string, error) {
	return f.byToken[token], nil
}

// ctxWithToken attaches an auth_token like the gateway does.
func ctxWithToken(token string) context.Context {
	return metadata.NewIncomingContext(context.Background(),
		metadata.Pairs(authTokenHeader, token))
}

func newTestServer(objs *fakeObjects, pres *fakePresence, auth *fakeAuth) *Server {
	return NewServer(objs, pres, auth, 5000, 15*time.Minute)
}

// --- LoadMap -------------------------------------------------------------

func TestLoadMap_PassesRadiusAndPoint(t *testing.T) {
	objs := &fakeObjects{within: []postgres.Object{
		{ID: "obj-1", ObjectType: "PARK", Longitude: 10, Latitude: 20},
	}}
	pres := &fakePresence{counts: map[string]int{"obj-1": 3}}
	auth := &fakeAuth{byToken: map[string]string{"tok": "user-A"}}
	s := newTestServer(objs, pres, auth)

	resp, err := s.LoadMap(ctxWithToken("tok"),
		&mapv1.LoadMapRequest{Longitude: 12.34, Latitude: 56.78})
	if err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	if objs.lastRadius != 5000 {
		t.Errorf("radius = %d, want 5000 (ST_DWithin 5km)", objs.lastRadius)
	}
	if objs.lastLon != 12.34 || objs.lastLat != 56.78 {
		t.Errorf("point = (%v,%v), want (12.34,56.78)", objs.lastLon, objs.lastLat)
	}
	if len(resp.Objects) != 1 || resp.Objects[0].VisitorCount != 3 {
		t.Errorf("visitor_count not surfaced: %+v", resp.Objects)
	}
	if resp.Code != 0 {
		t.Errorf("code = %d, want 0", resp.Code)
	}
}

// Privacy: strangers get counts, only the caller's friends get names, and the
// raw visitor set is never exposed.
func TestLoadMap_PrivacyFiltering(t *testing.T) {
	objs := &fakeObjects{within: []postgres.Object{{ID: "park", ObjectType: "DOG_PARK"}}}
	pres := &fakePresence{
		counts:  map[string]int{"park": 5},
		friends: map[string][]string{"park": {"friend-1", "friend-2"}},
	}
	auth := &fakeAuth{byToken: map[string]string{"tok": "caller"}}
	s := newTestServer(objs, pres, auth)

	resp, err := s.LoadMap(ctxWithToken("tok"), &mapv1.LoadMapRequest{})
	if err != nil {
		t.Fatalf("LoadMap: %v", err)
	}
	got := resp.Objects[0]
	if got.VisitorCount != 5 {
		t.Errorf("visitor_count = %d, want 5 (for everyone)", got.VisitorCount)
	}
	if len(got.FriendIdsHere) != 2 {
		t.Errorf("friend_ids_here = %v, want the 2 friends only", got.FriendIdsHere)
	}
}

func TestLoadMap_RequiresToken(t *testing.T) {
	s := newTestServer(&fakeObjects{}, &fakePresence{}, &fakeAuth{})
	_, err := s.LoadMap(context.Background(), &mapv1.LoadMapRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("no token: code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestLoadMap_InvalidToken(t *testing.T) {
	s := newTestServer(&fakeObjects{}, &fakePresence{}, &fakeAuth{byToken: map[string]string{}})
	_, err := s.LoadMap(ctxWithToken("bogus"), &mapv1.LoadMapRequest{})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("bad token: code = %v, want Unauthenticated", status.Code(err))
	}
}

func TestLoadMap_OutOfRangeCoords(t *testing.T) {
	auth := &fakeAuth{byToken: map[string]string{"tok": "u"}}
	s := newTestServer(&fakeObjects{}, &fakePresence{}, auth)
	_, err := s.LoadMap(ctxWithToken("tok"), &mapv1.LoadMapRequest{Longitude: 999, Latitude: 0})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("out-of-range lon: code = %v, want InvalidArgument", status.Code(err))
	}
}

// --- GetMapObject --------------------------------------------------------

func TestGetMapObject_NotFound(t *testing.T) {
	objs := &fakeObjects{byID: map[string]postgres.Object{}}
	auth := &fakeAuth{byToken: map[string]string{"tok": "u"}}
	s := newTestServer(objs, &fakePresence{}, auth)

	_, err := s.GetMapObject(ctxWithToken("tok"), &mapv1.GetMapObjectRequest{Id: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

func TestGetMapObject_Success(t *testing.T) {
	objs := &fakeObjects{byID: map[string]postgres.Object{
		"p1": {ID: "p1", ObjectType: "DOG_BEACH", Longitude: 1, Latitude: 2},
	}}
	pres := &fakePresence{counts: map[string]int{"p1": 7}, friends: map[string][]string{"p1": {"f1"}}}
	auth := &fakeAuth{byToken: map[string]string{"tok": "u"}}
	s := newTestServer(objs, pres, auth)

	resp, err := s.GetMapObject(ctxWithToken("tok"), &mapv1.GetMapObjectRequest{Id: "p1"})
	if err != nil {
		t.Fatalf("GetMapObject: %v", err)
	}
	if resp.Object.ObjectType != mapv1.ObjectType_DOG_BEACH {
		t.Errorf("object_type = %v, want DOG_BEACH", resp.Object.ObjectType)
	}
	if resp.Object.VisitorCount != 7 || len(resp.Object.FriendIdsHere) != 1 {
		t.Errorf("presence view wrong: %+v", resp.Object)
	}
}

// --- ChangeMapObjectStatus ----------------------------------------------

// Path 1, steps 5-6: mark VISITING -> count goes up; mark NOT_VISITING -> down.
func TestChangeStatus_VisitingThenNotVisiting(t *testing.T) {
	objs := &fakeObjects{byID: map[string]postgres.Object{"park": {ID: "park", ObjectType: "PARK"}}}
	pres := &fakePresence{counts: map[string]int{"park": 0}}
	auth := &fakeAuth{byToken: map[string]string{"tok": "user-A"}}
	s := newTestServer(objs, pres, auth)

	resp, err := s.ChangeMapObjectStatus(ctxWithToken("tok"),
		&mapv1.ChangeMapObjectStatusRequest{Id: "park", Action: mapv1.PresenceAction_VISITING})
	if err != nil {
		t.Fatalf("VISITING: %v", err)
	}
	if resp.Object.VisitorCount != 1 {
		t.Errorf("after VISITING count = %d, want 1", resp.Object.VisitorCount)
	}
	if len(pres.markedVisiting) != 1 || pres.markedVisiting[0].user != "user-A" {
		t.Fatalf("MarkVisiting not called for token owner: %+v", pres.markedVisiting)
	}
	if pres.markedVisiting[0].ttl != 15*time.Minute {
		t.Errorf("ttl = %v, want 15m", pres.markedVisiting[0].ttl)
	}

	resp, err = s.ChangeMapObjectStatus(ctxWithToken("tok"),
		&mapv1.ChangeMapObjectStatusRequest{Id: "park", Action: mapv1.PresenceAction_NOT_VISITING})
	if err != nil {
		t.Fatalf("NOT_VISITING: %v", err)
	}
	if resp.Object.VisitorCount != 0 {
		t.Errorf("after NOT_VISITING count = %d, want 0", resp.Object.VisitorCount)
	}
}

// The acting user is the token owner, NEVER a body id. There is no user_id field
// in the request, so we prove the marked user matches the token, not anything else.
func TestChangeStatus_ActingUserFromTokenNotBody(t *testing.T) {
	objs := &fakeObjects{byID: map[string]postgres.Object{"park": {ID: "park", ObjectType: "PARK"}}}
	pres := &fakePresence{counts: map[string]int{}}
	auth := &fakeAuth{byToken: map[string]string{"tokB": "user-B"}}
	s := newTestServer(objs, pres, auth)

	_, err := s.ChangeMapObjectStatus(ctxWithToken("tokB"),
		&mapv1.ChangeMapObjectStatusRequest{Id: "park", Action: mapv1.PresenceAction_VISITING})
	if err != nil {
		t.Fatalf("ChangeMapObjectStatus: %v", err)
	}
	if pres.markedVisiting[0].user != "user-B" {
		t.Errorf("acting user = %q, want user-B (from token)", pres.markedVisiting[0].user)
	}
}

func TestChangeStatus_UnknownObject(t *testing.T) {
	objs := &fakeObjects{byID: map[string]postgres.Object{}}
	auth := &fakeAuth{byToken: map[string]string{"tok": "u"}}
	s := newTestServer(objs, &fakePresence{}, auth)

	_, err := s.ChangeMapObjectStatus(ctxWithToken("tok"),
		&mapv1.ChangeMapObjectStatusRequest{Id: "nope", Action: mapv1.PresenceAction_VISITING})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code = %v, want NotFound", status.Code(err))
	}
}

func TestChangeStatus_RequiresToken(t *testing.T) {
	s := newTestServer(&fakeObjects{}, &fakePresence{}, &fakeAuth{})
	_, err := s.ChangeMapObjectStatus(context.Background(),
		&mapv1.ChangeMapObjectStatusRequest{Id: "park", Action: mapv1.PresenceAction_VISITING})
	if status.Code(err) != codes.Unauthenticated {
		t.Fatalf("code = %v, want Unauthenticated", status.Code(err))
	}
}

// A store error maps to the code+message error envelope (Internal).
func TestLoadMap_StoreErrorMapsToInternal(t *testing.T) {
	objs := &fakeObjects{withinErr: errors.New("db down")}
	auth := &fakeAuth{byToken: map[string]string{"tok": "u"}}
	s := newTestServer(objs, &fakePresence{}, auth)

	_, err := s.LoadMap(ctxWithToken("tok"), &mapv1.LoadMapRequest{})
	if status.Code(err) != codes.Internal {
		t.Fatalf("code = %v, want Internal", status.Code(err))
	}
}
