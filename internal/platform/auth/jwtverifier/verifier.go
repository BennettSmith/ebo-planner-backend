package jwtverifier

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/platform/config"
)

var (
	ErrUnauthorized = errors.New("unauthorized")
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

type Verifier struct {
	cfg    config.JWTConfig
	client *http.Client
	clock  Clock

	mu          sync.Mutex
	keysByKID   map[string]*rsa.PublicKey
	lastRefresh time.Time
	refreshing  bool
	refreshDone chan struct{}
}

func New(cfg config.JWTConfig) *Verifier {
	return NewWithOptions(cfg, nil, nil)
}

func NewWithOptions(cfg config.JWTConfig, httpClient *http.Client, clock Clock) *Verifier {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: cfg.HTTPTimeout}
	}
	if clock == nil {
		clock = realClock{}
	}
	return &Verifier{
		cfg:       cfg,
		client:    httpClient,
		clock:     clock,
		keysByKID: map[string]*rsa.PublicKey{},
	}
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Iss string          `json:"iss"`
	Sub string          `json:"sub"`
	Aud json.RawMessage `json:"aud"`
	Exp *int64          `json:"exp"`
	Nbf *int64          `json:"nbf"`
}

// Verify verifies a JWT and returns the authenticated subject from the `sub` claim.
//
// Verification:
// - RS256 signature using keys fetched from JWKS
// - iss, aud, exp, and nbf (when present)
func (v *Verifier) Verify(ctx context.Context, token string) (string, error) {
	h, claims, signingInput, sig, err := parseJWT(token)
	if err != nil {
		return "", ErrUnauthorized
	}
	if h.Alg != "RS256" || h.Kid == "" {
		return "", ErrUnauthorized
	}

	// Refresh rules:
	// - refresh periodically (rotation), even if kid exists in cache
	// - refresh on unknown kid, bounded by min refresh interval
	if err := v.maybeRefresh(ctx, h.Kid); err != nil {
		return "", ErrUnauthorized
	}

	pub := v.getKey(h.Kid)
	if pub == nil {
		return "", ErrUnauthorized
	}
	if err := verifyRS256(pub, signingInput, sig); err != nil {
		return "", ErrUnauthorized
	}
	if err := v.validateClaims(claims); err != nil {
		return "", ErrUnauthorized
	}
	if claims.Sub == "" {
		return "", ErrUnauthorized
	}
	return claims.Sub, nil
}

func (v *Verifier) validateClaims(c jwtClaims) error {
	now := v.clock.Now()
	skew := v.cfg.ClockSkew

	if c.Iss != v.cfg.Issuer {
		return fmt.Errorf("iss mismatch")
	}
	if !audMatches(c.Aud, v.cfg.Audience) {
		return fmt.Errorf("aud mismatch")
	}
	if c.Exp == nil {
		return fmt.Errorf("missing exp")
	}
	if now.After(time.Unix(*c.Exp, 0).Add(skew)) {
		return fmt.Errorf("token expired")
	}
	if c.Nbf != nil && now.Before(time.Unix(*c.Nbf, 0).Add(-skew)) {
		return fmt.Errorf("token not yet valid")
	}
	return nil
}

func (v *Verifier) getKey(kid string) *rsa.PublicKey {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.keysByKID[kid]
}

func (v *Verifier) maybeRefresh(ctx context.Context, kid string) error {
	now := v.clock.Now()

	v.mu.Lock()
	needsIntervalRefresh := !v.lastRefresh.IsZero() && v.cfg.JWKSRefreshInterval > 0 && now.Sub(v.lastRefresh) >= v.cfg.JWKSRefreshInterval
	unknownKid := v.keysByKID[kid] == nil
	allowedUnknownKidRefresh := v.lastRefresh.IsZero() || v.cfg.JWKSMinRefreshInterval <= 0 || now.Sub(v.lastRefresh) >= v.cfg.JWKSMinRefreshInterval
	shouldRefresh := needsIntervalRefresh || (unknownKid && allowedUnknownKidRefresh)

	if !shouldRefresh {
		v.mu.Unlock()
		return nil
	}

	// Deduplicate concurrent refresh attempts.
	if v.refreshing {
		ch := v.refreshDone
		v.mu.Unlock()
		select {
		case <-ch:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	v.refreshing = true
	v.refreshDone = make(chan struct{})
	ch := v.refreshDone
	v.mu.Unlock()

	err := v.refresh(ctx)

	v.mu.Lock()
	v.refreshing = false
	close(ch)
	v.mu.Unlock()

	return err
}

func (v *Verifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return err
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return fmt.Errorf("jwks fetch failed: status=%d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	keys, err := parseJWKS(body)
	if err != nil {
		return err
	}

	v.mu.Lock()
	v.keysByKID = keys
	v.lastRefresh = v.clock.Now()
	v.mu.Unlock()

	return nil
}

func parseJWT(token string) (jwtHeader, jwtClaims, string, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return jwtHeader{}, jwtClaims{}, "", nil, fmt.Errorf("bad jwt parts")
	}
	headerB, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return jwtHeader{}, jwtClaims{}, "", nil, err
	}
	claimsB, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return jwtHeader{}, jwtClaims{}, "", nil, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return jwtHeader{}, jwtClaims{}, "", nil, err
	}
	var h jwtHeader
	if err := json.Unmarshal(headerB, &h); err != nil {
		return jwtHeader{}, jwtClaims{}, "", nil, err
	}
	var c jwtClaims
	if err := json.Unmarshal(claimsB, &c); err != nil {
		return jwtHeader{}, jwtClaims{}, "", nil, err
	}
	return h, c, parts[0] + "." + parts[1], sig, nil
}

func verifyRS256(pub *rsa.PublicKey, signingInput string, sig []byte) error {
	sum := sha256.Sum256([]byte(signingInput))
	return rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig)
}

func audMatches(raw json.RawMessage, expected string) bool {
	if len(raw) == 0 {
		return false
	}
	// aud can be a string or an array of strings.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s == expected
	}
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, v := range arr {
			if v == expected {
				return true
			}
		}
	}
	return false
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func parseJWKS(b []byte) (map[string]*rsa.PublicKey, error) {
	var set jwks
	if err := json.Unmarshal(b, &set); err != nil {
		return nil, err
	}
	out := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		nb, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		eb, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		e := new(big.Int).SetBytes(eb).Int64()
		if e <= 0 || e > int64(^uint(0)>>1) {
			return nil, fmt.Errorf("invalid jwk exponent")
		}
		out[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nb),
			E: int(e),
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no usable jwks keys")
	}
	return out, nil
}
