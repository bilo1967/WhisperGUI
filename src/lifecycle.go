package main

import (
	"context"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func (a *application) shutdownSignal() <-chan struct{} { return a.done }
func (a *application) shutdown() {
	if !a.closing.CompareAndSwap(false, true) {
		return
	}
	a.job.mu.Lock()
	cancel := a.job.cancel
	a.job.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.modelTask.mu.Lock()
	mc := a.modelTask.cancel
	a.modelTask.mu.Unlock()
	if mc != nil {
		mc()
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = a.server.Shutdown(ctx)
		close(a.done)
	}()
}

// WebSocket ownership belongs to this tab: no cross-process window manipulation.
// Only a localhost origin matching the server is permitted.
var clientUpgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "http://"+r.Host
	},
}

func (a *application) clientSocket(w http.ResponseWriter, r *http.Request) {
	// The browser sends the random session token as a query parameter on WS upgrade.
	if r.URL.Query().Get("token") != a.token {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	conn, err := clientUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	a.clientMu.Lock()
	a.connectedClients++
	a.clientEverConnected = true
	a.lastClientGone = time.Time{}
	a.clientMu.Unlock()
	defer func() {
		_ = conn.Close()
		a.clientMu.Lock()
		a.connectedClients--
		if a.connectedClients == 0 {
			a.lastClientGone = time.Now()
		}
		a.clientMu.Unlock()
	}()

	// Ping/Pong happens at transport level, unaffected by background-tab JS timers.
	const pongWait = 60 * time.Second
	const pingPeriod = 20 * time.Second
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(pongWait)) })
	stop := make(chan struct{})
	go func() {
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(5*time.Second)); err != nil {
					_ = conn.Close()
					return
				}
			case <-stop:
				return
			}
		}
	}()
	defer close(stop)
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
	}
}

func (a *application) workInProgress() bool {
	a.job.mu.Lock()
	jobActive := a.job.state == "converting" || a.job.state == "transcribing"
	a.job.mu.Unlock()
	a.modelTask.mu.Lock()
	modelActive := a.modelTask.busy
	a.modelTask.mu.Unlock()
	return jobActive || modelActive
}

func (a *application) monitorClient() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	started := time.Now()
	for {
		select {
		case <-a.done:
			return
		case <-ticker.C:
		}
		a.clientMu.Lock()
		connected := a.connectedClients
		ever := a.clientEverConnected
		goneAt := a.lastClientGone
		a.clientMu.Unlock()
		if connected != 0 || a.workInProgress() {
			continue
		}
		if !ever {
			// Give the browser enough time to start and establish its first connection.
			if time.Since(started) > 90*time.Second {
				a.shutdown()
				return
			}
			continue
		}
		if !goneAt.IsZero() && time.Since(goneAt) >= time.Duration(a.config.Server.DisconnectGraceSeconds)*time.Second {
			a.shutdown()
			return
		}
	}
}
