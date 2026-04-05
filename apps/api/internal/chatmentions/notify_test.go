package chatmentions

import (
	"testing"

	"github.com/google/uuid"
)

func TestParseMentionIDs(t *testing.T) {
	u1 := uuid.New()
	u2 := uuid.New()
	r1 := uuid.New()
	content := "hello <@" + u1.String() + "> and <@" + u2.String() + "> role <@&" + r1.String() + ">"
	users, roles := ParseMentionIDs(content)
	if len(users) != 2 || len(roles) != 1 {
		t.Fatalf("unexpected parse counts users=%d roles=%d", len(users), len(roles))
	}
}
