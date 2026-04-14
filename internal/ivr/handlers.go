// Package ivr exposes a small service-agnostic session API so any IVR
// front-end (Jambonz, Twilio, Asterisk, a Pi with a DTMF decoder, a test
// script) can drive on-channel digit entry with four REST calls:
//
//	POST   /api/ivr/sessions             -> {session_id}
//	POST   /api/ivr/sessions/{id}/digit  -> {"digit": "5"}
//	POST   /api/ivr/sessions/{id}/submit -> finalise early (optional)
//	DELETE /api/ivr/sessions/{id}
//
// At most MaxConcurrent sessions (default 3) are accepted at once; the
// patent allowed multiple simultaneous callers on-screen and the frontend
// overlay is already wired up for three. Each session broadcasts
// `dial_update` WebSocket events so the channel shows the phone icon,
// digit stream, and accept/reject feedback (patent FIG. 1 step 32,
// "DISPLAY SELECTION #").
package ivr

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alexkinch/retromusicbox/internal/catalogue"
	"github.com/alexkinch/retromusicbox/internal/queue"
	"github.com/alexkinch/retromusicbox/internal/ws"
)

const (
	MaxConcurrent    = 3
	CodeLength       = 3
	SessionTTL       = 30 * time.Second
	ResultLingerTime = 4 * time.Second
)

type sessionStatus string

const (
	statusDialling sessionStatus = "dialling"
	statusSuccess  sessionStatus = "success"
	statusFail     sessionStatus = "fail"
)

type session struct {
	ID         string        `json:"id"`
	Digits     string        `json:"digits"`
	Status     sessionStatus `json:"status"`
	CallerID   string        `json:"caller_id,omitempty"`
	CreatedAt  time.Time     `json:"-"`
	UpdatedAt  time.Time     `json:"-"`
	submitting bool
}

type Handler struct {
	mu             sync.Mutex
	sessions       map[string]*session
	catalogue      *catalogue.Service
	queue          *queue.Service
	hub            *ws.Hub
	onChange       func()
	postSubmitHold time.Duration
}

func NewHandler(cat *catalogue.Service, q *queue.Service, hub *ws.Hub, onChange func(), postSubmitHold time.Duration) *Handler {
	h := &Handler{
		sessions:       make(map[string]*session),
		catalogue:      cat,
		queue:          q,
		hub:            hub,
		onChange:       onChange,
		postSubmitHold: postSubmitHold,
	}
	go h.reaper()
	return h
}

func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/ivr/sessions", h.handleCreate)
	mux.HandleFunc("POST /api/ivr/sessions/{id}/digit", h.handleDigit)
	mux.HandleFunc("POST /api/ivr/sessions/{id}/submit", h.handleSubmit)
	mux.HandleFunc("DELETE /api/ivr/sessions/{id}", h.handleDelete)
	mux.HandleFunc("GET /api/ivr/sessions/{id}", h.handleGet)
}

// --- handlers ---------------------------------------------------------------

type createRequest struct {
	CallerID string `json:"caller_id,omitempty"`
}

type createResponse struct {
	SessionID string `json:"session_id"`
	ExpiresIn int    `json:"expires_in_seconds"`
}

func (h *Handler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	h.mu.Lock()
	if h.activeCount() >= MaxConcurrent {
		h.mu.Unlock()
		writeError(w, http.StatusTooManyRequests, "all lines are busy")
		return
	}
	id := newSessionID()
	now := time.Now()
	s := &session{
		ID:        id,
		Digits:    "",
		Status:    statusDialling,
		CallerID:  req.CallerID,
		CreatedAt: now,
		UpdatedAt: now,
	}
	h.sessions[id] = s
	h.mu.Unlock()

	h.broadcastDialUpdate()

	writeJSON(w, http.StatusCreated, createResponse{
		SessionID: id,
		ExpiresIn: int(SessionTTL / time.Second),
	})
}

type digitRequest struct {
	Digit string `json:"digit"`
}

type sessionResponse struct {
	ID     string        `json:"id"`
	Digits string        `json:"digits"`
	Status sessionStatus `json:"status"`
	// Populated on status=success
	Code     string `json:"code,omitempty"`
	Title    string `json:"title,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Position int    `json:"position,omitempty"`
	// Populated on status=fail
	Reason string `json:"reason,omitempty"`
}

func (h *Handler) handleDigit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req digitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	d := strings.TrimSpace(req.Digit)
	if len(d) != 1 || d[0] < '0' || d[0] > '9' {
		if d == "#" || d == "*" {
			// Treat # as submit, * as clear
			if d == "#" {
				h.submit(w, id)
				return
			}
			h.mu.Lock()
			s, ok := h.sessions[id]
			if !ok {
				h.mu.Unlock()
				writeError(w, http.StatusNotFound, "session not found")
				return
			}
			s.Digits = ""
			s.Status = statusDialling
			s.UpdatedAt = time.Now()
			resp := h.snapshotLocked(s)
			h.mu.Unlock()
			h.broadcastDialUpdate()
			writeJSON(w, http.StatusOK, resp)
			return
		}
		writeError(w, http.StatusBadRequest, "digit must be one of 0-9, #, *")
		return
	}

	h.mu.Lock()
	s, ok := h.sessions[id]
	if !ok {
		h.mu.Unlock()
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if s.Status != statusDialling {
		resp := h.snapshotLocked(s)
		h.mu.Unlock()
		writeJSON(w, http.StatusConflict, resp)
		return
	}
	if len(s.Digits) >= CodeLength {
		// Already full — ignore extra digits
		resp := h.snapshotLocked(s)
		h.mu.Unlock()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	s.Digits += d
	s.UpdatedAt = time.Now()
	full := len(s.Digits) == CodeLength
	resp := h.snapshotLocked(s)
	h.mu.Unlock()

	h.broadcastDialUpdate()

	if full {
		h.submit(w, id)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleSubmit(w http.ResponseWriter, r *http.Request) {
	h.submit(w, r.PathValue("id"))
}

func (h *Handler) submit(w http.ResponseWriter, id string) {
	h.mu.Lock()
	s, ok := h.sessions[id]
	if !ok {
		h.mu.Unlock()
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	if s.Status != statusDialling || s.submitting {
		resp := h.snapshotLocked(s)
		h.mu.Unlock()
		writeJSON(w, http.StatusOK, resp)
		return
	}
	s.submitting = true
	code := s.Digits
	callerID := s.CallerID
	if callerID == "" {
		callerID = "ivr:" + id
	}
	h.mu.Unlock()

	if len(code) != CodeLength {
		h.finalise(id, statusFail, "incomplete code", nil, 0)
		writeJSON(w, http.StatusOK, h.snapshot(id))
		return
	}

	entry, err := h.catalogue.GetByCode(code)
	if err != nil || entry == nil {
		h.finalise(id, statusFail, "unknown code", nil, 0)
		writeJSON(w, http.StatusOK, h.snapshot(id))
		return
	}

	_, position, err := h.queue.Add(code, callerID)
	if err != nil {
		h.finalise(id, statusFail, err.Error(), entry, 0)
		writeJSON(w, http.StatusOK, h.snapshot(id))
		return
	}

	// Successful request — hold the completed code on screen (and on the phone
	// line) for a configurable beat before confirming with "Thanx!". Mirrors
	// the deliberate pause the original Box had on accepted requests only;
	// rejections flash "Try again" instantly.
	if h.postSubmitHold > 0 {
		time.Sleep(h.postSubmitHold)
	}

	h.finalise(id, statusSuccess, "", entry, position)
	if h.onChange != nil {
		h.onChange()
	}
	writeJSON(w, http.StatusOK, h.snapshot(id))
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h.mu.Lock()
	_, ok := h.sessions[id]
	if ok {
		delete(h.sessions, id)
	}
	h.mu.Unlock()
	if ok {
		h.broadcastDialUpdate()
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	snap := h.snapshot(id)
	if snap.ID == "" {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

// --- internals --------------------------------------------------------------

func (h *Handler) finalise(id string, status sessionStatus, reason string, entry *catalogue.Entry, position int) {
	h.mu.Lock()
	s, ok := h.sessions[id]
	if !ok {
		h.mu.Unlock()
		return
	}
	s.Status = status
	s.UpdatedAt = time.Now()
	_ = reason
	_ = entry
	_ = position
	h.mu.Unlock()

	h.broadcastDialUpdate()
}

func (h *Handler) snapshot(id string) sessionResponse {
	h.mu.Lock()
	defer h.mu.Unlock()
	s, ok := h.sessions[id]
	if !ok {
		return sessionResponse{}
	}
	return h.snapshotLocked(s)
}

func (h *Handler) snapshotLocked(s *session) sessionResponse {
	return sessionResponse{
		ID:     s.ID,
		Digits: s.Digits,
		Status: s.Status,
	}
}

func (h *Handler) activeCount() int {
	n := 0
	for _, s := range h.sessions {
		if s.Status == statusDialling {
			n++
		}
	}
	return n
}

// reaper evicts idle sessions. Dialling sessions expire after SessionTTL,
// finalised sessions linger for ResultLingerTime so the on-screen overlay
// has time to display the accept/reject state before disappearing.
func (h *Handler) reaper() {
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	for range t.C {
		h.sweep()
	}
}

func (h *Handler) sweep() {
	now := time.Now()
	changed := false
	h.mu.Lock()
	for id, s := range h.sessions {
		ttl := SessionTTL
		if s.Status != statusDialling {
			ttl = ResultLingerTime
		}
		if now.Sub(s.UpdatedAt) > ttl {
			delete(h.sessions, id)
			changed = true
		}
	}
	h.mu.Unlock()
	if changed {
		h.broadcastDialUpdate()
	}
}

type dialUpdate struct {
	Type    string         `json:"type"`
	Callers []dialSnapshot `json:"callers"`
}

type dialSnapshot struct {
	ID     string        `json:"id"`
	Digits string        `json:"digits"`
	Status sessionStatus `json:"status"`
}

func (h *Handler) broadcastDialUpdate() {
	if h.hub == nil {
		return
	}
	h.mu.Lock()
	callers := make([]dialSnapshot, 0, len(h.sessions))
	for _, s := range h.sessions {
		callers = append(callers, dialSnapshot{ID: s.ID, Digits: s.Digits, Status: s.Status})
	}
	h.mu.Unlock()
	h.hub.BroadcastEvent(dialUpdate{Type: "dial_update", Callers: callers})
}

func newSessionID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

func writeJSON(w http.ResponseWriter, code int, body interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
