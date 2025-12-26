package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"log"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"
)

// Tiny dev-only JWT issuer + JWKS server.
//
// This is NOT a full OIDC provider. It exists to support local development against
// real RS256 JWT verification (iss/aud/exp + JWKS).

type jwk struct {
	Kty string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwks struct {
	Keys []jwk `json:"keys"`
}

func main() {
	port := getenv("PORT", "5556")
	issuer := getenv("ISSUER", "http://devjwt:5556")
	audience := getenv("AUDIENCE", "east-bay-overland")
	kid := getenv("KID", "dev-kid-1")
	ttl := getenvDuration("TTL", 30*time.Minute)

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("generate key: %v", err)
	}

	jwksJSON, err := marshalJWKS(priv.PublicKey, kid)
	if err != nil {
		log.Fatalf("marshal jwks: %v", err)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Common JWKS path used by many providers.
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(jwksJSON)
	})

	// Mint a JWT:
	//   GET /token?sub=dev|alice
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		sub := strings.TrimSpace(r.URL.Query().Get("sub"))
		if sub == "" {
			http.Error(w, "missing sub", http.StatusBadRequest)
			return
		}

		now := time.Now().UTC()
		token, err := mintRS256JWT(priv, kid, issuer, audience, sub, now, ttl)
		if err != nil {
			http.Error(w, "failed to mint token", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token": token,
			"sub":   sub,
			"iss":   issuer,
			"aud":   audience,
			"exp":   now.Add(ttl).Unix(),
		})
	})

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("devjwt listening on :%s (iss=%s aud=%s kid=%s ttl=%s)", port, issuer, audience, kid, ttl)
	log.Fatal(srv.ListenAndServe())
}

func marshalJWKS(pub rsa.PublicKey, kid string) ([]byte, error) {
	enc := base64.RawURLEncoding
	n := enc.EncodeToString(pub.N.Bytes())
	e := big.NewInt(int64(pub.E)).Bytes() // big-endian unsigned
	eStr := enc.EncodeToString(e)
	set := jwks{
		Keys: []jwk{{
			Kty: "RSA",
			Use: "sig",
			Alg: "RS256",
			Kid: kid,
			N:   n,
			E:   eStr,
		}},
	}
	return json.Marshal(set)
}

func mintRS256JWT(priv *rsa.PrivateKey, kid, iss, aud, sub string, now time.Time, ttl time.Duration) (string, error) {
	header := map[string]any{
		"alg": "RS256",
		"typ": "JWT",
		"kid": kid,
	}
	claims := map[string]any{
		"iss": iss,
		"aud": aud,
		"sub": sub,
		"exp": now.Add(ttl).Unix(),
		"nbf": now.Add(-5 * time.Second).Unix(), // small skew tolerance for local use
	}

	hb, err := json.Marshal(header)
	if err != nil {
		return "", err
	}
	cb, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}

	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(hb) + "." + enc.EncodeToString(cb)
	sum := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, priv, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

func getenv(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func getenvDuration(k string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(k))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
