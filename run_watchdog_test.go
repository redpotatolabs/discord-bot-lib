package botlib

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/gorilla/websocket"
)

// fakeGateway points discordgo at a local gateway whose connections are
// served by handle (n counts connections from 1) and returns an App wired
// to it.
func fakeGateway(t *testing.T, handle func(c *websocket.Conn, n int32)) *App {
	t.Helper()
	var conns atomic.Int32
	upgrader := websocket.Upgrader{}
	mux := http.NewServeMux()
	var srv *httptest.Server
	mux.HandleFunc("/api/gateway", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"url":"ws` + strings.TrimPrefix(srv.URL, "http") + `/gw"}`))
	})
	mux.HandleFunc("/gw/", func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		handle(c, conns.Add(1))
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	prev := discordgo.EndpointGateway
	discordgo.EndpointGateway = srv.URL + "/api/gateway"
	t.Cleanup(func() { discordgo.EndpointGateway = prev })

	app, err := New("test-token", 0)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

// identifyAndReady plays the normal handshake: Hello, read Identify, READY.
func identifyAndReady(c *websocket.Conn, heartbeatMsec int) bool {
	hello := `{"op":10,"d":{"heartbeat_interval":` + strconv.Itoa(heartbeatMsec) + `}}`
	if c.WriteMessage(websocket.TextMessage, []byte(hello)) != nil {
		return false
	}
	if _, _, err := c.ReadMessage(); err != nil {
		return false
	}
	ready := `{"op":0,"s":1,"t":"READY","d":{"session_id":"s","user":{"id":"1","username":"test"}}}`
	return c.WriteMessage(websocket.TextMessage, []byte(ready)) == nil
}

// TestRun_WatchdogExitsOnOp7Deadlock reproduces the discordgo v0.29.0
// deadlock: the first connection drops, and the gateway answers the
// reconnect with Op 7, so Open calls CloseWithCode while holding the session
// lock. Run must return ErrGatewayStalled instead of hanging forever.
func TestRun_WatchdogExitsOnOp7Deadlock(t *testing.T) {
	var conns atomic.Int32
	app := fakeGateway(t, func(c *websocket.Conn, n int32) {
		conns.Store(n)
		if n > 1 {
			_ = c.WriteMessage(websocket.TextMessage, []byte(`{"op":7,"d":null}`))
			return
		}
		if identifyAndReady(c, 45000) {
			time.Sleep(200 * time.Millisecond) // then drop the connection
		}
	})

	result := make(chan error, 1)
	// Stall must exceed the 1s discordgo holds the lock in a normal Close.
	go func() { result <- app.Run(t.Context(), RunConfig{WatchdogStall: 2 * time.Second}) }()

	select {
	case err := <-result:
		if !errors.Is(err, ErrGatewayStalled) {
			t.Fatalf("Run err = %v, want ErrGatewayStalled", err)
		}
		if !strings.Contains(err.Error(), "session lock held") {
			t.Errorf("Run err = %q, want lock-held cause", err)
		}
		if n := conns.Load(); n != 2 {
			t.Errorf("gateway saw %d connections, want 2 (initial + reconnect)", n)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("Run did not return; watchdog failed to detect the deadlock")
	}
}

func TestRun_WatchdogLeavesHealthyGatewayAlone(t *testing.T) {
	app := fakeGateway(t, func(c *websocket.Conn, _ int32) {
		if !identifyAndReady(c, 50) {
			return
		}
		for { // ACK every heartbeat
			if _, _, err := c.ReadMessage(); err != nil {
				return
			}
			if c.WriteMessage(websocket.TextMessage, []byte(`{"op":11}`)) != nil {
				return
			}
		}
	})

	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	if err := app.Run(ctx, RunConfig{WatchdogStall: 500 * time.Millisecond}); err != nil {
		t.Fatalf("Run err = %v, want clean shutdown", err)
	}
}
