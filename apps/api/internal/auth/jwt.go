package auth

import (
	"crypto"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	_ "crypto/sha256" // register SHA-256 for crypto.Hash
	_ "crypto/sha512" // register SHA-384 and SHA-512
	"encoding/base64"
	"encoding/json"
	"errors"
	"math/big"
	"strings"
)

const maxTokenBytes = 64 << 10

// algorithm describes a supported JWS signature algorithm. Requiring the key
// type and curve to match the algorithm prevents algorithm-confusion attacks;
// "none" and HMAC algorithms are deliberately unsupported.
type algorithm struct {
	hash crypto.Hash
	kty  string
	crv  string
	size int // ECDSA coordinate and signature-half length in bytes
}

var algorithms = map[string]algorithm{
	"RS256": {crypto.SHA256, "RSA", "", 0},
	"RS384": {crypto.SHA384, "RSA", "", 0},
	"RS512": {crypto.SHA512, "RSA", "", 0},
	"ES256": {crypto.SHA256, "EC", "P-256", 32},
	"ES384": {crypto.SHA384, "EC", "P-384", 48},
	"ES512": {crypto.SHA512, "EC", "P-521", 66},
}

type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
	Crv string `json:"crv"`
	X   string `json:"x"`
	Y   string `json:"y"`
}

type verifyKey struct {
	kid string
	alg string // optional restriction from the JWK
	kty string
	crv string
	key crypto.PublicKey
}

// parseKeys converts a JWK set to signature keys, skipping keys this package
// cannot use instead of rejecting the whole set.
func parseKeys(set []jwk) []verifyKey {
	keys := make([]verifyKey, 0, len(set))
	for _, k := range set {
		if k.Use != "" && k.Use != "sig" {
			continue
		}
		// Assign only non-nil pointers: a nil *T stored in an interface is not nil.
		var key crypto.PublicKey
		switch k.Kty {
		case "RSA":
			if pub := rsaKey(k); pub != nil {
				key = pub
			}
		case "EC":
			if pub := ecKey(k); pub != nil {
				key = pub
			}
		}
		if key != nil {
			keys = append(keys, verifyKey{kid: k.Kid, alg: k.Alg, kty: k.Kty, crv: k.Crv, key: key})
		}
	}
	return keys
}

func rsaKey(k jwk) *rsa.PublicKey {
	n, errN := base64.RawURLEncoding.DecodeString(k.N)
	e, errE := base64.RawURLEncoding.DecodeString(k.E)
	// Three bytes covers the universal exponent 65537 and keeps E within int32.
	if errN != nil || errE != nil || len(e) == 0 || len(e) > 3 {
		return nil
	}
	modulus := new(big.Int).SetBytes(n)
	exponent := int(new(big.Int).SetBytes(e).Int64())
	if modulus.BitLen() < 2048 || exponent < 3 || exponent%2 == 0 {
		return nil
	}
	return &rsa.PublicKey{N: modulus, E: exponent}
}

func ecKey(k jwk) *ecdsa.PublicKey {
	var curve elliptic.Curve
	var check ecdh.Curve
	var size int
	switch k.Crv {
	case "P-256":
		curve, check, size = elliptic.P256(), ecdh.P256(), 32
	case "P-384":
		curve, check, size = elliptic.P384(), ecdh.P384(), 48
	case "P-521":
		curve, check, size = elliptic.P521(), ecdh.P521(), 66
	default:
		return nil
	}
	x, errX := base64.RawURLEncoding.DecodeString(k.X)
	y, errY := base64.RawURLEncoding.DecodeString(k.Y)
	if errX != nil || errY != nil || len(x) != size || len(y) != size {
		return nil
	}
	// Reject points that are not on the curve.
	if _, err := check.NewPublicKey(append(append([]byte{4}, x...), y...)); err != nil {
		return nil
	}
	return &ecdsa.PublicKey{Curve: curve, X: new(big.Int).SetBytes(x), Y: new(big.Int).SetBytes(y)}
}

type jwtHeader struct {
	Alg  string          `json:"alg"`
	Kid  string          `json:"kid"`
	Crit json.RawMessage `json:"crit"`
}

// parseJWT splits a compact JWS and decodes its header and payload. The
// signature is not checked; call verifySignature before trusting the payload.
func parseJWT(token string) (jwtHeader, []byte, error) {
	var header jwtHeader
	if len(token) > maxTokenBytes {
		return header, nil, errors.New("ID token is too large")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return header, nil, errors.New("ID token is not a compact JWS")
	}
	rawHeader, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(rawHeader, &header) != nil {
		return header, nil, errors.New("ID token header is malformed")
	}
	if header.Crit != nil {
		return header, nil, errors.New("ID token uses unsupported critical header parameters")
	}
	if _, ok := algorithms[header.Alg]; !ok {
		return header, nil, errors.New("ID token uses an unsupported signature algorithm")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return header, nil, errors.New("ID token payload is malformed")
	}
	return header, payload, nil
}

// verifySignature checks the token against any candidate key compatible with
// the header's algorithm.
func verifySignature(token string, header jwtHeader, keys []verifyKey) error {
	alg := algorithms[header.Alg]
	dot := strings.LastIndexByte(token, '.')
	signature, err := base64.RawURLEncoding.DecodeString(token[dot+1:])
	if dot < 0 || err != nil {
		return errors.New("ID token signature is malformed")
	}
	h := alg.hash.New()
	h.Write([]byte(token[:dot]))
	digest := h.Sum(nil)
	for _, k := range keys {
		if k.kty != alg.kty || (k.alg != "" && k.alg != header.Alg) || (alg.crv != "" && k.crv != alg.crv) {
			continue
		}
		switch pub := k.key.(type) {
		case *rsa.PublicKey:
			if rsa.VerifyPKCS1v15(pub, alg.hash, digest, signature) == nil {
				return nil
			}
		case *ecdsa.PublicKey:
			if len(signature) == 2*alg.size {
				r := new(big.Int).SetBytes(signature[:alg.size])
				s := new(big.Int).SetBytes(signature[alg.size:])
				if ecdsa.Verify(pub, digest, r, s) {
					return nil
				}
			}
		}
	}
	return errors.New("ID token signature is invalid")
}

// audience accepts the JWT "aud" claim as a string or an array of strings.
type audience []string

func (a *audience) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*a = audience{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return errors.New("aud must be a string or an array of strings")
	}
	*a = many
	return nil
}

func (a audience) contains(value string) bool {
	for _, item := range a {
		if item == value {
			return true
		}
	}
	return false
}
