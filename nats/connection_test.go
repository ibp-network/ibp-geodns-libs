package nats

import (
	"testing"

	natsio "github.com/nats-io/nats.go"
)

func TestCloneNatsMsgDeepCopiesPayload(t *testing.T) {
	original := &natsio.Msg{
		Subject: "consensus.propose",
		Reply:   "_INBOX.test",
		Header: natsio.Header{
			"X-Test": []string{"a"},
		},
		Data: []byte(`{"ok":true}`),
	}

	cloned := cloneNatsMsg(original)
	if cloned == original {
		t.Fatalf("expected a distinct message instance")
	}
	if cloned.Subject != original.Subject || cloned.Reply != original.Reply {
		t.Fatalf("expected subject and reply to be preserved")
	}

	cloned.Data[0] = 'X'
	cloned.Header["X-Test"][0] = "b"

	if string(original.Data) != `{"ok":true}` {
		t.Fatalf("expected original data to remain unchanged, got %q", string(original.Data))
	}
	if original.Header["X-Test"][0] != "a" {
		t.Fatalf("expected original headers to remain unchanged, got %q", original.Header["X-Test"][0])
	}
}
