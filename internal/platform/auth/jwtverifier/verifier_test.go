package jwtverifier_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/Overland-East-Bay/trip-planner-api/internal/platform/auth/jwks_testutil"
	"github.com/Overland-East-Bay/trip-planner-api/internal/platform/auth/jwtverifier"
	"github.com/Overland-East-Bay/trip-planner-api/internal/platform/config"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestVerifier_Verify_ValidToken(t *testing.T) {
	t.Parallel()

	jwksSrv, setKeys := jwks_testutil.NewRotatingJWKSServer()
	defer jwksSrv.Close()

	kp, err := jwks_testutil.GenerateRSAKeypair("kid-1")
	if err != nil {
		t.Fatalf("GenerateRSAKeypair: %v", err)
	}
	setKeys([]jwks_testutil.Keypair{kp})

	clk := &fakeClock{now: time.Unix(1700000000, 0)}
	cfg := config.JWTConfig{
		Issuer:                 "test-iss",
		Audience:               "test-aud",
		JWKSURL:                jwksSrv.URL,
		ClockSkew:              0,
		JWKSRefreshInterval:    10 * time.Minute,
		JWKSMinRefreshInterval: 0,
		HTTPTimeout:            2 * time.Second,
	}

	v := jwtverifier.NewWithOptions(cfg, nil, clk)

	jwt, err := jwks_testutil.MintRS256JWT(kp, cfg.Issuer, cfg.Audience, "member-123", clk.Now(), 5*time.Minute, nil)
	if err != nil {
		t.Fatalf("MintRS256JWT: %v", err)
	}

	sub, err := v.Verify(context.Background(), jwt)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if sub != "member-123" {
		t.Fatalf("sub mismatch: got %q", sub)
	}
}

func TestVerifier_Verify_Expired(t *testing.T) {
	t.Parallel()

	jwksSrv, setKeys := jwks_testutil.NewRotatingJWKSServer()
	defer jwksSrv.Close()

	kp, _ := jwks_testutil.GenerateRSAKeypair("kid-1")
	setKeys([]jwks_testutil.Keypair{kp})

	clk := &fakeClock{now: time.Unix(1700000000, 0)}
	cfg := config.JWTConfig{
		Issuer:                 "test-iss",
		Audience:               "test-aud",
		JWKSURL:                jwksSrv.URL,
		ClockSkew:              0,
		JWKSRefreshInterval:    10 * time.Minute,
		JWKSMinRefreshInterval: 0,
		HTTPTimeout:            2 * time.Second,
	}
	v := jwtverifier.NewWithOptions(cfg, nil, clk)

	jwt, _ := jwks_testutil.MintRS256JWT(kp, cfg.Issuer, cfg.Audience, "member-123", clk.Now(), -1*time.Minute, nil)
	if _, err := v.Verify(context.Background(), jwt); err == nil {
		t.Fatalf("expected error")
	}
}

func TestVerifier_Verify_WrongIssuerOrAudience(t *testing.T) {
	t.Parallel()

	jwksSrv, setKeys := jwks_testutil.NewRotatingJWKSServer()
	defer jwksSrv.Close()

	kp, _ := jwks_testutil.GenerateRSAKeypair("kid-1")
	setKeys([]jwks_testutil.Keypair{kp})

	clk := &fakeClock{now: time.Unix(1700000000, 0)}
	cfg := config.JWTConfig{
		Issuer:                 "test-iss",
		Audience:               "test-aud",
		JWKSURL:                jwksSrv.URL,
		ClockSkew:              0,
		JWKSRefreshInterval:    10 * time.Minute,
		JWKSMinRefreshInterval: 0,
		HTTPTimeout:            2 * time.Second,
	}
	v := jwtverifier.NewWithOptions(cfg, nil, clk)

	jwtWrongIss, _ := jwks_testutil.MintRS256JWT(kp, "wrong-iss", cfg.Audience, "member-123", clk.Now(), 5*time.Minute, nil)
	if _, err := v.Verify(context.Background(), jwtWrongIss); err == nil {
		t.Fatalf("expected error for wrong iss")
	}

	jwtWrongAud, _ := jwks_testutil.MintRS256JWT(kp, cfg.Issuer, "wrong-aud", "member-123", clk.Now(), 5*time.Minute, nil)
	if _, err := v.Verify(context.Background(), jwtWrongAud); err == nil {
		t.Fatalf("expected error for wrong aud")
	}
}

func TestVerifier_Verify_BadSignature(t *testing.T) {
	t.Parallel()

	jwksSrv, setKeys := jwks_testutil.NewRotatingJWKSServer()
	defer jwksSrv.Close()

	kp, _ := jwks_testutil.GenerateRSAKeypair("kid-1")
	setKeys([]jwks_testutil.Keypair{kp})

	clk := &fakeClock{now: time.Unix(1700000000, 0)}
	cfg := config.JWTConfig{
		Issuer:                 "test-iss",
		Audience:               "test-aud",
		JWKSURL:                jwksSrv.URL,
		ClockSkew:              0,
		JWKSRefreshInterval:    10 * time.Minute,
		JWKSMinRefreshInterval: 0,
		HTTPTimeout:            2 * time.Second,
	}
	v := jwtverifier.NewWithOptions(cfg, nil, clk)

	// Mint a JWT with a different private key than what's in JWKS.
	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	otherKP := jwks_testutil.Keypair{Kid: "kid-1", Private: other}
	jwt, _ := jwks_testutil.MintRS256JWT(otherKP, cfg.Issuer, cfg.Audience, "member-123", clk.Now(), 5*time.Minute, nil)
	if _, err := v.Verify(context.Background(), jwt); err == nil {
		t.Fatalf("expected error")
	}
}

func TestVerifier_Verify_JWKSRotation_OldKidRejected_NewKidAccepted(t *testing.T) {
	t.Parallel()

	jwksSrv, setKeys := jwks_testutil.NewRotatingJWKSServer()
	defer jwksSrv.Close()

	k1, _ := jwks_testutil.GenerateRSAKeypair("kid-1")
	k2, _ := jwks_testutil.GenerateRSAKeypair("kid-2")
	setKeys([]jwks_testutil.Keypair{k1})

	clk := &fakeClock{now: time.Unix(1700000000, 0)}
	cfg := config.JWTConfig{
		Issuer:                 "test-iss",
		Audience:               "test-aud",
		JWKSURL:                jwksSrv.URL,
		ClockSkew:              0,
		JWKSRefreshInterval:    1 * time.Second,
		JWKSMinRefreshInterval: 0,
		HTTPTimeout:            2 * time.Second,
	}
	v := jwtverifier.NewWithOptions(cfg, nil, clk)

	jwt1, _ := jwks_testutil.MintRS256JWT(k1, cfg.Issuer, cfg.Audience, "member-123", clk.Now(), 5*time.Minute, nil)
	if _, err := v.Verify(context.Background(), jwt1); err != nil {
		t.Fatalf("expected jwt1 to verify: %v", err)
	}

	// Rotate: JWKS now only contains kid-2.
	setKeys([]jwks_testutil.Keypair{k2})
	clk.Advance(2 * time.Second) // force interval refresh on next Verify call.

	// Old kid should be rejected after refresh.
	if _, err := v.Verify(context.Background(), jwt1); err == nil {
		t.Fatalf("expected jwt1 to be rejected after rotation")
	}

	jwt2, _ := jwks_testutil.MintRS256JWT(k2, cfg.Issuer, cfg.Audience, "member-456", clk.Now(), 5*time.Minute, nil)
	sub, err := v.Verify(context.Background(), jwt2)
	if err != nil {
		t.Fatalf("expected jwt2 to verify: %v", err)
	}
	if sub != "member-456" {
		t.Fatalf("sub mismatch: got %q", sub)
	}
}
