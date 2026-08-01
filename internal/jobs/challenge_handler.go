package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/joaojsr/shiori-server/internal/platform/browser"
	"github.com/joaojsr/shiori-server/internal/platform/httpserver"
)

const (
	challengeReadLimit  = 4096
	challengePongWait   = 45 * time.Second
	challengePingPeriod = 20 * time.Second
	challengeWriteWait  = 5 * time.Second
)

var challengeUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 64 * 1024,
	// A nil CheckOrigin uses Gorilla's safe same-origin default.
	EnableCompression: false,
}

type ChallengeHandler struct {
	bp browser.Provider
	cm *browser.ChallengeManager
}

func NewChallengeHandler(bp browser.Provider, cm *browser.ChallengeManager) *ChallengeHandler {
	return &ChallengeHandler{bp: bp, cm: cm}
}

func (h *ChallengeHandler) RegisterRoutes(r chi.Router) {
	r.Get("/challenges/assets/client.js", h.serveClientJS)
	r.Get("/challenges/{token}", h.serveHTML)
	r.Get("/challenges/{token}/status", h.getStatus)
	r.Get("/challenges/{token}/ws", h.serveWS)
	r.Post("/challenges/{token}/complete", h.completeChallenge)
	r.Delete("/challenges/{token}", h.cancelChallenge)
}

const screencastHTML = `<!doctype html>
<html lang="pt-BR">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>Shiori — verificação humana</title>
  <style>
    :root { color-scheme: dark; font-family: system-ui, sans-serif; }
    body { margin: 0; min-height: 100vh; background: #141518; color: #f4f4f5; display: grid; grid-template-rows: auto 1fr auto; }
    header, footer { padding: 12px 16px; background: #1d1f24; }
    header { display: flex; justify-content: space-between; gap: 16px; align-items: center; }
    #status { margin: 0; font-weight: 650; }
    #timer { color: #c5c7ce; font-variant-numeric: tabular-nums; }
    main { min-width: 0; min-height: 0; overflow: hidden; background: #fff; }
    canvas { display: block; width: 100%; height: 100%; background: white; outline: none; touch-action: none; }
    canvas:focus-visible { box-shadow: 0 0 0 3px #8ab4ff; }
    footer { display: flex; justify-content: flex-end; gap: 8px; }
    button { border: 0; border-radius: 6px; padding: 10px 16px; font: inherit; font-weight: 650; cursor: pointer; }
    #cancel { background: #353840; color: #fff; }
    #complete { background: #8ab4ff; color: #101114; }
    button:disabled { opacity: .55; cursor: wait; }
  </style>
  <script src="/api/v1/challenges/assets/client.js" defer></script>
</head>
<body>
  <header><p id="status" role="status" aria-live="polite">Conectando à sessão segura…</p><span id="timer"></span></header>
  <main><canvas id="screencast" tabindex="0" aria-label="Navegador remoto para concluir a verificação"></canvas></main>
  <footer><button id="cancel" type="button">Cancelar</button><button id="complete" type="button" disabled>Verificar e continuar</button></footer>
</body>
</html>`

const challengeClientJS = `(() => {
  'use strict';
  const parts = location.pathname.split('/').filter(Boolean);
  const token = parts[parts.length - 1];
  const base = '/api/v1/challenges/' + encodeURIComponent(token);
  const canvas = document.getElementById('screencast');
  const viewport = canvas.parentElement;
  const context = canvas.getContext('2d');
  const status = document.getElementById('status');
  const timer = document.getElementById('timer');
  const completeButton = document.getElementById('complete');
  const cancelButton = document.getElementById('cancel');
  let socket;
  let imageURL;
  let expiresAt;
  let movePending = false;
  let lastMove;
  let viewportPending;

  const measureViewport = () => {
    const rect = viewport.getBoundingClientRect();
    viewportPending = {
      type: 'viewport',
      width: Math.max(320, Math.min(3840, Math.round(rect.width))),
      height: Math.max(240, Math.min(2160, Math.round(rect.height)))
    };
    send(viewportPending);
  };

  const setStatus = (message) => { status.textContent = message; };
  const send = (payload) => {
    if (socket && socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify(payload));
  };
  const coordinates = (event) => {
    const rect = canvas.getBoundingClientRect();
    return {
      x: Math.round((event.clientX - rect.left) * canvas.width / rect.width),
      y: Math.round((event.clientY - rect.top) * canvas.height / rect.height)
    };
  };
  const buttonName = (button) => button === 0 ? 'left' : button === 1 ? 'middle' : button === 2 ? 'right' : 'none';

  const connect = () => {
    const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
    socket = new WebSocket(protocol + '//' + location.host + base + '/ws');
    socket.binaryType = 'blob';
    socket.onopen = () => {
      setStatus('Sessão conectada. Conclua somente a verificação apresentada pela fonte.');
      completeButton.disabled = false;
      measureViewport();
      canvas.focus();
    };
    socket.onmessage = (event) => {
      if (!(event.data instanceof Blob)) return;
      const nextURL = URL.createObjectURL(event.data);
      const image = new Image();
      image.onload = () => {
        canvas.width = image.naturalWidth;
        canvas.height = image.naturalHeight;
        context.drawImage(image, 0, 0);
        if (imageURL) URL.revokeObjectURL(imageURL);
        imageURL = nextURL;
      };
      image.onerror = () => URL.revokeObjectURL(nextURL);
      image.src = nextURL;
    };
    socket.onclose = () => {
      completeButton.disabled = true;
      if (!document.body.dataset.finished) setStatus('Conexão encerrada. Reabra esta página para reconectar.');
    };
  };

  canvas.addEventListener('pointerdown', (event) => {
    canvas.setPointerCapture(event.pointerId);
    canvas.focus();
    send({ type: 'mousePressed', ...coordinates(event), button: buttonName(event.button) });
  });
  canvas.addEventListener('pointerup', (event) => send({ type: 'mouseReleased', ...coordinates(event), button: buttonName(event.button) }));
  canvas.addEventListener('pointermove', (event) => {
    lastMove = event;
    if (movePending) return;
    movePending = true;
    requestAnimationFrame(() => {
      movePending = false;
      if (lastMove) send({ type: 'mouseMoved', ...coordinates(lastMove), button: 'none' });
    });
  });
  canvas.addEventListener('wheel', (event) => {
    event.preventDefault();
    send({ type: 'mouseWheel', ...coordinates(event), button: 'none', delta_x: event.deltaX, delta_y: event.deltaY });
  }, { passive: false });
  canvas.addEventListener('keydown', (event) => {
    event.preventDefault();
    send({ type: 'keyDown', key: event.key, code: event.code, text: event.key.length === 1 ? event.key : '', modifiers: (event.altKey ? 1 : 0) | (event.ctrlKey ? 2 : 0) | (event.metaKey ? 4 : 0) | (event.shiftKey ? 8 : 0) });
  });
  canvas.addEventListener('keyup', (event) => {
    event.preventDefault();
    send({ type: 'keyUp', key: event.key, code: event.code, modifiers: (event.altKey ? 1 : 0) | (event.ctrlKey ? 2 : 0) | (event.metaKey ? 4 : 0) | (event.shiftKey ? 8 : 0) });
  });
  canvas.addEventListener('contextmenu', (event) => event.preventDefault());
  new ResizeObserver(measureViewport).observe(viewport);

  completeButton.addEventListener('click', async () => {
    completeButton.disabled = true;
    setStatus('O backend está verificando se o desafio terminou…');
    const response = await fetch(base + '/complete', { method: 'POST', headers: { 'Accept': 'application/json' } });
    if (!response.ok) {
      const problem = await response.json().catch(() => ({}));
      setStatus(problem.detail || 'O desafio ainda está visível. Conclua-o antes de continuar.');
      completeButton.disabled = false;
      canvas.focus();
      return;
    }
    document.body.dataset.finished = 'true';
    setStatus('Verificação confirmada pelo backend. A extração será retomada.');
    completeButton.hidden = true;
    cancelButton.hidden = true;
    if (socket) socket.close(1000, 'completed');
  });

  cancelButton.addEventListener('click', async () => {
    cancelButton.disabled = true;
    await fetch(base, { method: 'DELETE' });
    document.body.dataset.finished = 'true';
    setStatus('Verificação cancelada.');
    completeButton.hidden = true;
    cancelButton.hidden = true;
    if (socket) socket.close(1000, 'cancelled');
  });

  const updateStatus = async () => {
    const response = await fetch(base + '/status', { headers: { 'Accept': 'application/json' } });
    if (!response.ok) return;
    const state = await response.json();
    expiresAt = Date.parse(state.expires_at);
    if (state.kind === 'login' && !document.body.dataset.finished) {
      setStatus('Sessão conectada. Entre no site; seus dados permanecem somente no navegador remoto.');
      completeButton.textContent = 'Confirmar login e continuar';
    }
    if (state.status === 'cancelled' || state.status === 'expired') {
      document.body.dataset.finished = 'true';
      setStatus(state.status === 'expired' ? 'A sessão expirou.' : 'A sessão foi cancelada.');
      completeButton.hidden = true;
      cancelButton.hidden = true;
      if (socket) socket.close();
    }
  };
  setInterval(() => {
    if (!expiresAt) return;
    const seconds = Math.max(0, Math.ceil((expiresAt - Date.now()) / 1000));
    timer.textContent = 'Expira em ' + Math.floor(seconds / 60) + ':' + String(seconds % 60).padStart(2, '0');
  }, 250);
  setInterval(updateStatus, 5000);
  updateStatus();
  connect();
})();`

func (h *ChallengeHandler) serveHTML(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if _, err := h.cm.Get(token); err != nil {
		respondChallengeError(w, err)
		return
	}
	setChallengeHeaders(w)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(screencastHTML))
}

func (h *ChallengeHandler) serveClientJS(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(challengeClientJS))
}

func (h *ChallengeHandler) getStatus(w http.ResponseWriter, r *http.Request) {
	view, err := h.cm.Get(chi.URLParam(r, "token"))
	if err != nil && !errors.Is(err, browser.ErrChallengeExpired) {
		respondChallengeError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, view)
}

func (h *ChallengeHandler) completeChallenge(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	sessionID, err := h.cm.BeginVerification(token)
	if err != nil {
		respondChallengeError(w, err)
		return
	}

	verifyCtx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	snapshot, err := h.bp.Snapshot(verifyCtx, sessionID)
	if err != nil {
		_ = h.cm.RejectVerification(token)
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusBadGateway, Title: "Challenge Verification Failed", Detail: "The browser session could not be verified."})
		return
	}
	if snapshot.UserAction {
		_ = h.cm.RejectVerification(token)
		detail := "The challenge is still visible in the browser session."
		if snapshot.UserActionKind == browser.UserActionLogin {
			detail = "The site still requires login. Complete authentication before continuing."
		} else if snapshot.UserActionKind == browser.UserActionBlocked {
			detail = "The source returned a definitive Cloudflare block, not an interactive challenge."
		}
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusConflict, Title: "User Action Still Required", Detail: detail})
		return
	}
	view, err := h.cm.Resolve(token)
	if err != nil {
		respondChallengeError(w, err)
		return
	}
	httpserver.RespondJSON(w, http.StatusOK, view)
}

func (h *ChallengeHandler) cancelChallenge(w http.ResponseWriter, r *http.Request) {
	if err := h.cm.Cancel(chi.URLParam(r, "token")); err != nil {
		respondChallengeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *ChallengeHandler) serveWS(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	sessionID, err := h.cm.AcquireController(token)
	if err != nil {
		respondChallengeError(w, err)
		return
	}
	defer h.cm.ReleaseController(token)

	conn, err := challengeUpgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(challengeReadLimit)
	_ = conn.SetReadDeadline(time.Now().Add(challengePongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(challengePongWait))
	})

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	frames := make(chan []byte, 2)
	inputEvents := make(chan browser.InputEvent, 64)

	go func() {
		defer cancel()
		_ = h.bp.Screencast(ctx, sessionID, frames, inputEvents)
	}()

	go writeChallengeStream(ctx, cancel, conn, frames)

	windowStarted := time.Now()
	eventCount := 0
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if time.Since(windowStarted) >= time.Second {
			windowStarted = time.Now()
			eventCount = 0
		}
		eventCount++
		if eventCount > 180 {
			continue
		}
		var event browser.InputEvent
		if err := json.Unmarshal(message, &event); err != nil || !event.Valid() {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case inputEvents <- event:
		}
	}
}

func writeChallengeStream(ctx context.Context, cancel context.CancelFunc, conn *websocket.Conn, frames <-chan []byte) {
	ticker := time.NewTicker(challengePingPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-frames:
			_ = conn.SetWriteDeadline(time.Now().Add(challengeWriteWait))
			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				cancel()
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(challengeWriteWait))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				cancel()
				return
			}
		}
	}
}

func setChallengeHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; img-src blob: data:; frame-ancestors 'self'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
}

func respondChallengeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, browser.ErrChallengeNotFound):
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusNotFound, Title: "Challenge Not Found", Detail: "The challenge token does not exist."})
	case errors.Is(err, browser.ErrChallengeExpired):
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusGone, Title: "Challenge Expired", Detail: "The human verification session has expired."})
	case errors.Is(err, browser.ErrChallengeInUse):
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusConflict, Title: "Challenge In Use", Detail: "Another controller is already connected to this challenge."})
	case errors.Is(err, browser.ErrChallengeCancelled):
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusGone, Title: "Challenge Cancelled", Detail: "The human verification session was cancelled."})
	default:
		httpserver.RespondError(w, httpserver.Problem{Status: http.StatusConflict, Title: "Invalid Challenge State", Detail: "The challenge cannot perform this operation in its current state."})
	}
}
