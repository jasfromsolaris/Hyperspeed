package chatai

import (
	"fmt"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestParseChatFileRefUUIDsOrderedUnique(t *testing.T) {
	id1 := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	id2 := uuid.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8")
	raw := fmt.Sprintf("see <#%s> and <#%s> again <#%s>", id1, id2, id1)
	got := parseChatFileRefUUIDsOrderedUnique(raw)
	if len(got) != 2 || got[0] != id1 || got[1] != id2 {
		t.Fatalf("got %v want [%s %s]", got, id1, id2)
	}
}

func TestReplaceChatFileRefsWithLabels(t *testing.T) {
	id := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	raw := fmt.Sprintf("read <#%s> please", id)
	labels := map[uuid.UUID]string{id: "notes.md"}
	got := replaceChatFileRefsWithLabels(raw, labels)
	if !strings.Contains(got, "[file: notes.md]") || strings.Contains(got, "<#") {
		t.Fatalf("got %q", got)
	}
	got2 := replaceChatFileRefsWithLabels(raw, nil)
	if got2 != "read [file] please" {
		t.Fatalf("got %q", got2)
	}
}

func TestStripChatMarkupTokens(t *testing.T) {
	uid := uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")
	fid := uuid.MustParse("6ba7b811-9dad-11d1-80b4-00c04fd430c8")
	s := fmt.Sprintf("hi <@%s> <#%s>", uid, fid)
	got := stripChatMarkupTokens(s)
	if got != "hi  " {
		t.Fatalf("got %q", got)
	}
}
