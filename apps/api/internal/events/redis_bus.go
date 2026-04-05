package events

import (
	"context"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type Bus struct {
	Rdb *redis.Client
}

func (b *Bus) Publish(ctx context.Context, orgID uuid.UUID, payload []byte) error {
	if b == nil || b.Rdb == nil {
		return nil
	}
	return b.Rdb.Publish(ctx, OrgChannel(orgID), payload).Err()
}
