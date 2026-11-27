package webhook

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/langyannb/devour-auth/internal/store"
)

func testStore(t *testing.T) (*store.Store, int64) {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	appID, err := st.CreateApp(ctx, "devour", "Devour", 1000)
	if err != nil {
		t.Fatal(err)
	}
	return st, appID
}

func testDispatcher(st *store.Store) *Dispatcher {
	d := New(st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	d.backoff = []time.Duration{0, 0} // 测试不等 1s、2s
	return d
}

func testHook(t *testing.T, st *store.Store, appID int64, url string) store.Webhook {
	t.Helper()
	h := store.Webhook{AppID: appID, URL: url, Secret: "s3cret", Events: "card.ban", Active: 1, CreatedAt: 1000}
	id, err := st.CreateWebhook(context.Background(), h)
	if err != nil {
		t.Fatal(err)
	}
	h.ID = id
	return h
}

func TestSendRetriesThenSucceeds(t *testing.T) {
	st, appID := testStore(t)
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	h := testHook(t, st, appID, ts.URL)
	testDispatcher(st).send(h, "card.ban", []byte(`{"event":"card.ban"}`))

	if n := atomic.LoadInt32(&calls); n != 2 {
		t.Fatalf("应请求 2 次，实际 %d", n)
	}
	list, err := st.ListWebhookDeliveries(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("应记录 1 条，实际 %d", len(list))
	}
	d := list[0]
	if d.Attempts != 2 || !d.OK || d.StatusCode != http.StatusOK || d.Error != "" {
		t.Fatalf("记录不对: %+v", d)
	}
	if d.WebhookID != h.ID || d.Event != "card.ban" || d.URL != ts.URL {
		t.Fatalf("快照字段不对: %+v", d)
	}
}

func TestSendRecordsFailureAfterAllAttempts(t *testing.T) {
	st, appID := testStore(t)
	var calls int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	h := testHook(t, st, appID, ts.URL)
	testDispatcher(st).send(h, "card.ban", []byte(`{"event":"card.ban"}`))

	if n := atomic.LoadInt32(&calls); n != 3 {
		t.Fatalf("应请求 3 次，实际 %d", n)
	}
	list, err := st.ListWebhookDeliveries(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("应记录 1 条，实际 %d", len(list))
	}
	d := list[0]
	if d.Attempts != 3 || d.OK || d.StatusCode != http.StatusServiceUnavailable || d.Error == "" {
		t.Fatalf("记录不对: %+v", d)
	}
}
