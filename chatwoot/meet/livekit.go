package meet

import (
	"context"
	"fmt"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

// createRoom creates a LiveKit room with MaxDuration.
// If the room already exists, it returns without error.
func createRoom(ctx context.Context, roomName string) error {
	cfg := getConfig()
	if cfg == nil {
		return ErrNotConfigured
	}

	roomClient := lksdk.NewRoomServiceClient(cfg.LiveKit.URL, cfg.LiveKit.APIKey, cfg.LiveKit.APISecret)

	_, err := roomClient.CreateRoom(ctx, &livekit.CreateRoomRequest{
		Name:             roomName,
		EmptyTimeout:     300, // 5 minutes after last participant leaves
		DepartureTimeout: uint32(cfg.RoomTimeout.Seconds()),
		MaxParticipants:  2,
	})
	if err != nil {
		return fmt.Errorf("meet: create room: %w", err)
	}
	return nil
}

// createParticipantToken generates a LiveKit JWT for a participant to join a room.
func createParticipantToken(roomName, identity, name string) (string, error) {
	cfg := getConfig()
	if cfg == nil {
		return "", ErrNotConfigured
	}

	at := auth.NewAccessToken(cfg.LiveKit.APIKey, cfg.LiveKit.APISecret)

	grant := &auth.VideoGrant{
		RoomJoin: true,
		Room:     roomName,
	}

	at.SetIdentity(identity).
		SetName(name).
		SetValidFor(cfg.TokenExpiry).
		SetVideoGrant(grant)

	return at.ToJWT()
}
