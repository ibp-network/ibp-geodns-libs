package nats

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

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

func TestSubscribeDoesNotSerializeCallbacks(t *testing.T) {
	srv := runRoleTestServer(t)

	libConn, err := natsio.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect library client: %v", err)
	}
	connectionMu.Lock()
	nc = libConn
	NC = libConn
	connectionMu.Unlock()
	t.Cleanup(func() {
		Disconnect()
	})

	publisher, err := natsio.Connect(srv.ClientURL())
	if err != nil {
		t.Fatalf("connect publisher client: %v", err)
	}
	t.Cleanup(func() {
		publisher.Close()
	})

	firstEntered := make(chan struct{}, 1)
	secondEntered := make(chan struct{}, 1)
	releaseFirst := make(chan struct{})
	var releaseOnce sync.Once
	var calls atomic.Int32

	sub, err := Subscribe("consensus.propose", func(m *natsio.Msg) {
		callNum := calls.Add(1)
		if callNum == 1 {
			firstEntered <- struct{}{}
			<-releaseFirst
			return
		}
		secondEntered <- struct{}{}
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	t.Cleanup(func() {
		_ = sub.Unsubscribe()
		releaseOnce.Do(func() { close(releaseFirst) })
	})
	if err := libConn.Flush(); err != nil {
		t.Fatalf("flush library subscription: %v", err)
	}

	if err := publisher.Publish("consensus.propose", []byte(`{"seq":1}`)); err != nil {
		t.Fatalf("publish first message: %v", err)
	}
	if err := publisher.Publish("consensus.propose", []byte(`{"seq":2}`)); err != nil {
		t.Fatalf("publish second message: %v", err)
	}
	if err := publisher.Flush(); err != nil {
		t.Fatalf("flush publisher: %v", err)
	}

	select {
	case <-firstEntered:
	case <-time.After(2 * time.Second):
		t.Fatal("first callback did not start")
	}

	select {
	case <-secondEntered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("second callback did not start while first callback was blocked")
	}

	releaseOnce.Do(func() { close(releaseFirst) })
}
