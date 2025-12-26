package jwks_testutil

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"time"
)

type Keypair struct {
	Kid     string
	Private *rsa.PrivateKey
}

func GenerateRSAKeypair(kid string) (Keypair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return Keypair{}, err
	}
	return Keypair{Kid: kid, Private: priv}, nil
}

// NewRotatingJWKSServer returns a JWKS server whose key set can be swapped at runtime.
//
// Use SetKeys to rotate keys.
func NewRotatingJWKSServer() (*httptest.Server, func(keys []Keypair)) {
	var jwksJSON atomic.Value // string
	jwksJSON.Store(`{"keys":[]}`)

	setKeys := func(keys []Keypair) {
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
		out := jwks{Keys: make([]jwk, 0, len(keys))}
		for _, kp := range keys {
			pub := kp.Private.PublicKey
			n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
			// e is a big-endian unsigned int.
			e := big.NewInt(int64(pub.E)).Bytes()
			eStr := base64.RawURLEncoding.EncodeToString(e)
			out.Keys = append(out.Keys, jwk{
				Kty: "RSA",
				Use: "sig",
				Alg: "RS256",
				Kid: kp.Kid,
				N:   n,
				E:   eStr,
			})
		}
		b, _ := json.Marshal(out)
		jwksJSON.Store(string(b))
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(jwksJSON.Load().(string)))
	}))

	return srv, setKeys
}

// MintRS256JWT creates a signed JWT using RS256 with the given keypair.
//
// aud may be either a string or []string.
func MintRS256JWT(kp Keypair, iss string, aud any, sub string, now time.Time, expDelta time.Duration, nbfDelta *time.Duration) (string, error) {
	header := map[string]any{
		"alg": "RS256",
		"typ": "JWT",
		"kid": kp.Kid,
	}

	claims := map[string]any{
		"iss": iss,
		"aud": aud,
		"sub": sub,
		"exp": now.Add(expDelta).Unix(),
	}
	if nbfDelta != nil {
		claims["nbf"] = now.Add(*nbfDelta).Unix()
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
	sig, err := rsa.SignPKCS1v15(rand.Reader, kp.Private, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}
