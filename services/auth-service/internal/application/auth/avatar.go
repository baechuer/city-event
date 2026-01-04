package auth

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

// UpdateAvatarURL parses the avatar URL to extract the image ID,
// updates the user record, and publishes an event if cleanup is needed.
func (s *Service) UpdateAvatarURL(ctx context.Context, userID, avatarURL string) (string, error) {
	var avatarID *string
	// Parse URL to extract ID
	// Assumed format: .../avatars/{uuid}/size.webp OR .../avatars/{uuid}
	if text := strings.TrimSpace(avatarURL); text != "" {
		parts := strings.Split(text, "/")
		for i := len(parts) - 1; i >= 0; i-- {
			p := parts[i]
			// Try to parse as UUID
			if _, err := uuid.Parse(p); err == nil {
				id := p
				avatarID = &id
				break
			}
		}
	}

	// Update DB
	prevID, err := s.users.UpdateAvatarImageID(ctx, userID, avatarID)
	if err != nil {
		return "", err
	}

	// Determine if cleanup is needed
	// If we had a previous ID, and it's different from the new one (or new is nil), prompt cleanup.
	if prevID != nil && *prevID != "" {
		isNew := true
		if avatarID != nil && *prevID == *avatarID {
			isNew = false
		}

		if isNew {
			// Best effort event publish
			_ = s.pub.PublishAvatarUpdated(ctx, AvatarUpdatedEvent{
				UserID:      userID,
				OldAvatarID: *prevID,
			})
		}
	}

	if prevID == nil {
		return "", nil
	}
	return *prevID, nil
}
