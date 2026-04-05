package chatmentions

import (
	"context"
	"encoding/json"
	"regexp"

	"github.com/google/uuid"

	"hyperspeed/api/internal/events"
	"hyperspeed/api/internal/store"
)

var (
	reMentionUser = regexp.MustCompile(`<@([0-9a-fA-F-]{36})>`)
	reMentionRole = regexp.MustCompile(`<@&([0-9a-fA-F-]{36})>`)
)

// ParseMentionIDs extracts user and role mention UUIDs from chat message content (<@uuid>, <@&roleuuid>).
func ParseMentionIDs(content string) (userIDs []uuid.UUID, roleIDs []uuid.UUID) {
	for _, m := range reMentionUser.FindAllStringSubmatch(content, -1) {
		if len(m) < 2 {
			continue
		}
		id, err := uuid.Parse(m[1])
		if err == nil {
			userIDs = append(userIDs, id)
		}
	}
	for _, m := range reMentionRole.FindAllStringSubmatch(content, -1) {
		if len(m) < 2 {
			continue
		}
		id, err := uuid.Parse(m[1])
		if err == nil {
			roleIDs = append(roleIDs, id)
		}
	}
	return userIDs, roleIDs
}

// NotifyMentionRecipients creates chat.mention inbox rows and publishes NotificationCreated for each recipient.
// The author is excluded. Recipients must pass UserCanAccessSpace (same rules as user-posted messages).
func NotifyMentionRecipients(ctx context.Context, s *store.Store, bus *events.Bus, orgID, spaceID, roomID, messageID, authorUserID uuid.UUID, content string) error {
	mentionedUsers, mentionedRoles := ParseMentionIDs(content)
	if len(mentionedUsers) == 0 && len(mentionedRoles) == 0 {
		return nil
	}
	recips := make(map[uuid.UUID]struct{})
	for _, id := range mentionedUsers {
		recips[id] = struct{}{}
	}
	for _, rid := range mentionedRoles {
		uids, err := s.ListUserIDsForRole(ctx, orgID, rid)
		if err != nil {
			return err
		}
		for _, id := range uids {
			recips[id] = struct{}{}
		}
	}
	delete(recips, authorUserID)

	for to := range recips {
		ok, err := s.UserCanAccessSpace(ctx, orgID, spaceID, to)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		payload, _ := json.Marshal(map[string]any{
			"space_id":     spaceID,
			"chat_room_id": roomID,
			"message_id":   messageID,
			"from_user_id": authorUserID,
		})
		n, err := s.CreateNotification(ctx, orgID, to, "chat.mention", payload)
		if err != nil {
			return err
		}
		if bus != nil {
			publishNotificationCreated(bus, orgID, spaceID, roomID, to, n)
		}
	}
	return nil
}

func publishNotificationCreated(bus *events.Bus, orgID, projectID, chatRoomID, to uuid.UUID, n store.Notification) {
	pid := projectID
	envPayload := map[string]any{
		"chat_room_id": chatRoomID,
		"payload": map[string]any{
			"user_id":      to,
			"notification": n,
		},
	}
	b, err := events.Marshal(events.NotificationCreated, orgID, &pid, envPayload)
	if err != nil {
		return
	}
	_ = bus.Publish(context.Background(), orgID, b)
}
