package ctxkey

import (
	"context"

	"github.com/google/uuid"
)

type userKey struct{}

func WithUserID(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, userKey{}, id)
}

func UserID(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(userKey{}).(uuid.UUID)
	return v, ok
}
