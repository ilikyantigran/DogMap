// Package clients holds Auth's gRPC clients to downstream services. Auth calls
// exactly one downstream: Profiles.CreateProfile, on successful Register.
package clients

import (
	"fmt"

	profilesv1 "auth-service/pkg/api/profiles/v1"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients struct {
	Profiles profilesv1.ProfilesClient

	conns []*grpc.ClientConn
}

// Dial creates a (lazy) gRPC client for Profiles. The connection is established
// on first use; Close shuts it down.
func Dial(profilesAddr string) (*Clients, error) {
	c := &Clients{}
	cc, err := grpc.NewClient(profilesAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()), // propagate trace context downstream
	)
	if err != nil {
		return nil, fmt.Errorf("dial profiles (%s): %w", profilesAddr, err)
	}
	c.conns = append(c.conns, cc)
	c.Profiles = profilesv1.NewProfilesClient(cc)
	return c, nil
}

func (c *Clients) Close() {
	for _, cc := range c.conns {
		_ = cc.Close()
	}
}
