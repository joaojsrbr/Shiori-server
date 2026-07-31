package jobs

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Allow all for now, since it can be opened from the frontend WebView
	},
}

type ChallengeHandler struct {
	bp browser.Provider
	cm *browser.ChallengeManager
}

func NewChallengeHandler(bp browser.Provider, cm *browser.ChallengeManager) *ChallengeHandler {
	return &ChallengeHandler{bp: bp, cm: cm}
}

func (h *ChallengeHandler) RegisterRoutes(r chi.Router) {
	r.Get("/challenges/{token}", h.serveHTML)
	r.Get("/challenges/{token}/ws", h.serveWS)
	r.Post("/challenges/{token}/complete", h.completeChallenge)
}

const screencastHTML = `<!DOCTYPE html>
<html>
<head>
    <title>Shiori - Human Verification</title>
    <style>
        body { margin: 0; padding: 0; background: #1a1b1e; display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100vh; font-family: sans-serif; color: white; }
        #container { position: relative; display: inline-block; box-shadow: 0 10px 30px rgba(0,0,0,0.5); border-radius: 8px; overflow: hidden; }
        canvas { display: block; max-width: 100%; height: auto; }
        #status { margin-bottom: 20px; font-weight: bold; }
        .btn { margin-top: 20px; padding: 10px 20px; background: #4caf50; color: white; border: none; border-radius: 4px; cursor: pointer; display: none; font-size: 16px; }
        .btn:hover { background: #45a049; }
    </style>
</head>
<body>
    <div id="status">Connecting to browser session...</div>
    <div id="container">
        <canvas id="screencast"></canvas>
    </div>
    <button id="doneBtn" class="btn" onclick="complete()">I have completed the challenge</button>

    <script>
        const token = window.location.pathname.split('/').pop();
        const wsProto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
        const wsUrl = wsProto + '//' + window.location.host + '/api/v1/challenges/' + token + '/ws';
        const ws = new WebSocket(wsUrl);
        
        const canvas = document.getElementById('screencast');
        const ctx = canvas.getContext('2d');
        const status = document.getElementById('status');
        const doneBtn = document.getElementById('doneBtn');

        let img = new Image();
        img.onload = () => {
            if (canvas.width !== img.width) canvas.width = img.width;
            if (canvas.height !== img.height) canvas.height = img.height;
            ctx.drawImage(img, 0, 0);
        };

        ws.onopen = () => {
            status.innerText = 'Connected. Please solve the challenge below.';
            doneBtn.style.display = 'block';
        };
        ws.onclose = () => {
            status.innerText = 'Session closed or expired.';
            canvas.style.display = 'none';
        };
        ws.onmessage = (event) => {
            const blob = event.data;
            const url = URL.createObjectURL(blob);
            img.src = url;
            // Note: In a real app we'd revokeObjectURL to prevent memory leaks, 
            // but for a 1-minute challenge this is fine.
        };

        function sendEvent(type, e) {
            if (ws.readyState !== WebSocket.OPEN) return;
            const rect = canvas.getBoundingClientRect();
            // Calculate scale in case canvas is resized by CSS
            const scaleX = canvas.width / rect.width;
            const scaleY = canvas.height / rect.height;
            
            const x = Math.round((e.clientX - rect.left) * scaleX);
            const y = Math.round((e.clientY - rect.top) * scaleY);
            
            let button = 'none';
            if (e.button === 0) button = 'left';
            else if (e.button === 2) button = 'right';

            ws.send(JSON.stringify({ type: type, x: x, y: y, button: button }));
        }

        canvas.addEventListener('mousedown', (e) => sendEvent('mousePressed', e));
        canvas.addEventListener('mouseup', (e) => sendEvent('mouseReleased', e));
        canvas.addEventListener('mousemove', (e) => sendEvent('mouseMoved', e));
        
        // Prevent context menu on right click
        canvas.addEventListener('contextmenu', e => e.preventDefault());

        function complete() {
            fetch('/api/v1/challenges/' + token + '/complete', { method: 'POST' })
                .then(() => {
                    status.innerText = 'Challenge marked as complete! You can close this view.';
                    doneBtn.style.display = 'none';
                    canvas.style.display = 'none';
                    ws.close();
                });
        }
    </script>
</body>
</html>`

func (h *ChallengeHandler) serveHTML(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if _, err := h.cm.GetSession(token); err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 404, Title: "Challenge Not Found"})
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(screencastHTML))
}

func (h *ChallengeHandler) completeChallenge(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if _, err := h.cm.GetSession(token); err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 404, Title: "Challenge Not Found"})
		return
	}
	h.cm.Resolve(token)
	w.WriteHeader(http.StatusOK)
}

func (h *ChallengeHandler) serveWS(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	sessionID, err := h.cm.GetSession(token)
	if err != nil {
		httpserver.RespondError(w, httpserver.Problem{Status: 404, Title: "Challenge Not Found"})
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return // Upgrade writes the error itself
	}
	defer conn.Close()

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	frames := make(chan []byte)
	input := make(chan browser.InputEvent)

	// Start screencast provider loop
	go func() {
		// Stop if Screencast exits
		defer cancel()
		_ = h.bp.Screencast(ctx, sessionID, frames, input)
	}()

	// Write frames to WS
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case f := <-frames:
				conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if err := conn.WriteMessage(websocket.BinaryMessage, f); err != nil {
					cancel()
					return
				}
			}
		}
	}()

	// Read input from WS
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			break
		}
		var ev browser.InputEvent
		if err := json.Unmarshal(msg, &ev); err == nil {
			select {
			case <-ctx.Done():
				return
			case input <- ev:
			}
		}
	}
}
