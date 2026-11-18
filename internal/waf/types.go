package waf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gymbag/internal/host"
)

// Action represents the firewall decision outcome.
type Action string

const (
	ActionAllow     Action = "allow"
	ActionChallenge Action = "challenge"
	ActionDeny      Action = "deny"
)

// Decision encapsulates a WAF policy verdict.
type Decision struct {
	Action     Action
	Reason     string
	StatusCode int
}

// Allow constructs an allow Decision.
func Allow() Decision {
	return Decision{Action: ActionAllow, StatusCode: http.StatusOK}
}

// Challenge constructs a PoW challenge Decision.
func Challenge() Decision {
	return Decision{Action: ActionChallenge, StatusCode: http.StatusForbidden}
}

// Deny constructs a block Decision with reason and HTTP status code.
func Deny(reason string, statusCode int) Decision {
	if statusCode == 0 {
		statusCode = http.StatusForbidden
	}
	return Decision{Action: ActionDeny, Reason: reason, StatusCode: statusCode}
}

// IncomingRequest is a transport-independent snapshot of an HTTP or WebSocket request.
type IncomingRequest struct {
	Method    string
	Host      string
	Path      string
	RawTarget string
	Query     string
	Headers   map[string]string
	ClientIP  string
	Cookies   map[string]string
}

// FromHTTPRequest builds an IncomingRequest from a net/http.Request and resolved client IP.
func FromHTTPRequest(r *http.Request, clientIP string) IncomingRequest {
	headers := make(map[string]string, len(r.Header)+1)
	for k, vals := range r.Header {
		if len(vals) > 0 {
			headers[strings.ToLower(k)] = strings.Join(vals, ", ")
		}
	}
	if r.Host != "" {
		headers["host"] = r.Host
	}

	cookies := make(map[string]string)
	if rawCookie := headers["cookie"]; rawCookie != "" {
		for _, part := range strings.Split(rawCookie, ";") {
			name, val, ok := strings.Cut(part, "=")
			if !ok {
				continue
			}
			cookies[strings.TrimSpace(name)] = strings.TrimSpace(val)
		}
	}

	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	query := r.URL.RawQuery

	rawTarget := r.RequestURI
	if rawTarget == "" {
		if ep := r.URL.EscapedPath(); ep != "" {
			rawTarget = ep
		} else {
			rawTarget = path
		}
		if query != "" {
			rawTarget += "?" + query
		}
	}

	return IncomingRequest{
		Method:    strings.ToUpper(r.Method),
		Host:      host.FromHeader(r.Host),
		Path:      path,
		RawTarget: rawTarget,
		Query:     query,
		Headers:   headers,
		ClientIP:  clientIP,
		Cookies:   cookies,
	}
}

// Rule evaluates a single WAF policy concern. Returning nil delegates to the next rule (OCP).
type Rule interface {
	Evaluate(ctx context.Context, req IncomingRequest) *Decision
}

// Detector classifies whether an IncomingRequest matches a traffic category (e.g., AI bot, suspicious).
type Detector interface {
	Match(ctx context.Context, req IncomingRequest) bool
}

// IntelProvider supplies IP threat intelligence and AI crawler IP lookups (ISP / DIP).
type IntelProvider interface {
	IsThreatIP(ctx context.Context, ipStr string) bool
	IsAIIP(ipStr string) bool
}

// AccessGate decides whether an IncomingRequest is allowed, challenged, or denied (ISP/DIP).
type AccessGate interface {
	Decide(ctx context.Context, req IncomingRequest) Decision
}

// Submission represents the browser's proof-of-work solution payload.
type Submission struct {
	ChallengeID string `json:"challenge_id"`
	Nonce       int64  `json:"nonce"`
	Verified    bool   `json:"verified"`
}

// ChallengeManager issues and verifies single-use proof-of-work challenges (ISP).
type ChallengeManager interface {
	Issue(ctx context.Context, hostName, clientIP string, difficulty, ttlSeconds int) (challengeID string, outDifficulty int, err error)
	Verify(ctx context.Context, hostName, clientIP string, sub Submission) (bool, error)
}

// ClearanceManager issues and validates signed clearance cookies (ISP).
type ClearanceManager interface {
	Issue(hostName, clientIP string, ttlSeconds int) string
	Valid(token, hostName, clientIP string) bool
}

// PageRenderer writes HTML challenge and block pages (ISP).
type PageRenderer interface {
	WriteChallenge(w http.ResponseWriter, challengeID string, difficulty int)
	WriteReject(w http.ResponseWriter, reason string, statusCode int)
}

// ParseSubmission strictly validates a JSON PoW verification payload without type coercion.
func ParseSubmission(body io.Reader) (Submission, int, error) {
	raw, err := io.ReadAll(io.LimitReader(body, 8192))
	if err != nil {
		return Submission{}, http.StatusBadRequest, fmt.Errorf("unable to read request body")
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return Submission{}, http.StatusBadRequest, fmt.Errorf("request body must not be empty")
	}

	type rawPayload struct {
		ChallengeID *json.RawMessage `json:"challenge_id"`
		Nonce       *json.RawMessage `json:"nonce"`
		Verified    *json.RawMessage `json:"verified"`
	}

	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	var wire rawPayload
	if err := dec.Decode(&wire); err != nil {
		var syntaxErr *json.SyntaxError
		if errors.As(err, &syntaxErr) || errors.Is(err, io.ErrUnexpectedEOF) {
			return Submission{}, http.StatusBadRequest, fmt.Errorf("invalid JSON syntax")
		}
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("invalid verification payload: %w", err)
	}
	if dec.More() {
		return Submission{}, http.StatusBadRequest, fmt.Errorf("unexpected trailing data after JSON object")
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return Submission{}, http.StatusBadRequest, fmt.Errorf("unexpected trailing tokens")
	}
	if wire.ChallengeID == nil || wire.Nonce == nil || wire.Verified == nil {
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("challenge_id, nonce, and verified are required")
	}

	// Strict string check for challenge_id
	if len(*wire.ChallengeID) < 2 || (*wire.ChallengeID)[0] != '"' {
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("challenge_id must be a string")
	}
	var challengeID string
	if err := json.Unmarshal(*wire.ChallengeID, &challengeID); err != nil {
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("challenge_id must be a valid string")
	}
	if len(challengeID) < 1 || len(challengeID) > 128 {
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("challenge_id length must be between 1 and 128")
	}

	// Strict integer check for nonce (no string, no fractional float)
	nonceBytes := bytes.TrimSpace(*wire.Nonce)
	if len(nonceBytes) == 0 || nonceBytes[0] == '"' || bytes.ContainsAny(nonceBytes, ".eE") {
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("nonce must be an integer")
	}
	var nonce int64
	if err := json.Unmarshal(nonceBytes, &nonce); err != nil {
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("nonce must be an integer")
	}
	if nonce < 0 || nonce > 50_000_000 {
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("nonce must be between 0 and 50000000")
	}

	// Strict boolean check for verified
	verBytes := bytes.TrimSpace(*wire.Verified)
	if !bytes.Equal(verBytes, []byte("true")) && !bytes.Equal(verBytes, []byte("false")) {
		return Submission{}, http.StatusUnprocessableEntity, fmt.Errorf("verified must be a boolean")
	}
	verified := bytes.Equal(verBytes, []byte("true"))

	return Submission{
		ChallengeID: challengeID,
		Nonce:       nonce,
		Verified:    verified,
	}, http.StatusOK, nil
}
