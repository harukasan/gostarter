package monitor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestHTTPMonitor(t *testing.T) {
	i := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i++
		if i%10 == 0 {
			http.Error(w, "ok", http.StatusOK)
			return
		}
		http.Error(w, "wrong", http.StatusOK)
	}))
	defer ts.Close()

	m := NewHTTPMonitor(&HTTPMonitorConfig{
		URL: &url.URL{
			Path: "/",
		},
		ExpectedStatusCode: http.StatusOK,
		ExpectedBody:       []byte("ok\n"),
		Interval:           10 * time.Millisecond,
		Retry:              10,
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	trigger := func(err error) error {
		t.Errorf("trigger called: %v", err)
		return nil
	}
	m.Run(ctx, "tcp", ts.Listener.Addr().String(), trigger)
}
