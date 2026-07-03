package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pg "profiles/internal/domain/postgres"
	profilesv1 "profiles/pkg/api/profiles/v1"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// TestTransport_EndToEndPrivacy proves the wiring the App assembles: a real gRPC
// server + grpc-gateway REST mux, with the auth_token header forwarded to gRPC
// metadata. It hits POST /v1/profiles/get over HTTP and asserts the friends-only
// PII rule holds through the whole edge (not just the handler in isolation), and
// that a request with no token is rejected — the token-derived-identity contract.
func TestTransport_EndToEndPrivacy(t *testing.T) {
	store := newFakeStore()
	cache := newFakeCache()
	store.profiles["u1"] = &pg.Profile{UserID: "u1", Login: "Test1", Name: "Ann", Email: "a@x.io", Phone: "+100"}
	store.profiles["u2"] = &pg.Profile{UserID: "u2", Login: "Test2", Name: "Bob", Email: "b@x.io", Phone: "+200"}
	cache.sessions["tok1"] = "u1"
	srv := NewServer(store, cache)

	// Real gRPC server on a loopback port.
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	gs := grpc.NewServer()
	profilesv1.RegisterProfilesServiceServer(gs, srv)
	go gs.Serve(lis)
	defer gs.Stop()

	// REST gateway forwarding auth_token → gRPC metadata (same matcher as App).
	ctx := context.Background()
	gwMux := runtime.NewServeMux(
		runtime.WithIncomingHeaderMatcher(func(key string) (string, bool) {
			if key == "Auth_token" || key == "Auth-Token" || key == "auth_token" {
				return "auth_token", true
			}
			return runtime.DefaultHeaderMatcher(key)
		}),
	)
	if err := profilesv1.RegisterProfilesServiceHandlerFromEndpoint(ctx, gwMux, lis.Addr().String(),
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(gwMux)
	defer ts.Close()

	get := func(token, target string) map[string]any {
		body, _ := json.Marshal(map[string]string{"user_id_target": target})
		req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/profiles/get", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("auth_token", token)
		}
		resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
		if err != nil {
			t.Fatalf("http: %v", err)
		}
		defer resp.Body.Close()
		var out map[string]any
		json.NewDecoder(resp.Body).Decode(&out)
		return out
	}

	// Non-friend over the real edge: reduced shape, no PII.
	out := get("tok1", "u2")
	if fmt.Sprint(out["hasPii"]) == "true" {
		t.Fatalf("non-friend leaked full shape: %v", out)
	}
	if out["email"] != nil && out["email"] != "" {
		t.Fatalf("PII leaked over HTTP edge: %v", out["email"])
	}
	if out["login"] != "Test2" {
		t.Fatalf("reduced shape missing login: %v", out)
	}

	// No token → unauthorized (identity comes from the token, not the body).
	out = get("", "u2")
	if fmt.Sprint(out["code"]) != "401" {
		t.Fatalf("no-token request must be unauthorized, got %v", out)
	}

	// Friends → full shape with PII.
	store.friendships[[2]string{"u1", "u2"}] = true
	store.friendships[[2]string{"u2", "u1"}] = true
	out = get("tok1", "u2")
	if out["email"] != "b@x.io" || out["phone"] != "+200" {
		t.Fatalf("friend must get PII over edge: %v", out)
	}
}
