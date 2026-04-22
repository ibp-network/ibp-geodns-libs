package stats

import (
	"testing"

	"github.com/ibp-network/ibp-geodns-libs/nats/core"
)

func TestHandleRequestRequiresReplyInbox(t *testing.T) {
	published := false
	replied := false

	deps := Dependencies{
		State: &core.NodeState{
			NodeID: "monitor-a",
		},
		Publish: func(subject string, data []byte) error {
			published = true
			return nil
		},
		PublishMsgWithReply: func(subject, reply string, data []byte) error {
			replied = true
			return nil
		},
	}

	HandleRequest(deps, "", []byte(`{"startTime":"2026-04-20T00:00:00Z","endTime":"2026-04-20T01:00:00Z"}`))

	if published {
		t.Fatal("expected missing-reply request not to publish on a shared subject")
	}
	if replied {
		t.Fatal("expected missing-reply request not to send a reply")
	}
}
