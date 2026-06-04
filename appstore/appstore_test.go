package appstore

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/spf13/viper"
)

// ---------- test helpers ----------

// makeChain builds a self-contained ECDSA cert chain: root (self-signed CA) →
// intermediate (signed by root) → leaf (signed by intermediate). Returns the
// root as PEM (for the verifier's trust anchor), the leaf private key (to sign a
// JWS), and the DER bytes of each cert (to populate the x5c header).
func makeChain(t *testing.T) (rootPEM []byte, leafKey *ecdsa.PrivateKey, leafDER, intDER, rootDER []byte) {
	t.Helper()
	mk := func() *ecdsa.PrivateKey {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("genkey: %v", err)
		}
		return k
	}
	now := time.Now()
	rootKey := mk()
	rootTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Root CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	rootDER, err := x509.CreateCertificate(rand.Reader, rootTmpl, rootTmpl, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("root cert: %v", err)
	}
	rootCert, _ := x509.ParseCertificate(rootDER)

	intKey := mk()
	intTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Test Intermediate CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	intDER, err = x509.CreateCertificate(rand.Reader, intTmpl, rootCert, &intKey.PublicKey, rootKey)
	if err != nil {
		t.Fatalf("intermediate cert: %v", err)
	}
	intCert, _ := x509.ParseCertificate(intDER)

	leafKey = mk()
	leafTmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(3),
		Subject:               pkix.Name{CommonName: "Test Leaf"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
	}
	leafDER, err = x509.CreateCertificate(rand.Reader, leafTmpl, intCert, &leafKey.PublicKey, intKey)
	if err != nil {
		t.Fatalf("leaf cert: %v", err)
	}

	rootPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: rootDER})
	return rootPEM, leafKey, leafDER, intDER, rootDER
}

func b64(der []byte) string { return base64.StdEncoding.EncodeToString(der) }

// signJWS signs claims with ES256 using leafKey and injects the x5c chain header.
func signJWS(t *testing.T, leafKey *ecdsa.PrivateKey, x5c []string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	tok.Header["x5c"] = x5c
	s, err := tok.SignedString(leafKey)
	if err != nil {
		t.Fatalf("sign jws: %v", err)
	}
	return s
}

// setTestIapKey installs a throwaway ECDSA P8 key into viper so GenerateJwtToken works.
func setTestIapKey(t *testing.T) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal p8: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	viper.Set("appstore.iap.key", string(pemBytes))
	viper.Set("appstore.iap.keyId", "TESTKEYID")
	viper.Set("appstore.iap.issuer", "test-issuer")
	t.Cleanup(viper.Reset)
}

// ---------- A: x5c leaf-indexing fix ----------

func TestVerifyPayloadWithRoot_ValidChainPasses(t *testing.T) {
	rootPEM, leafKey, leafDER, intDER, rootDER := makeChain(t)
	x5c := []string{b64(leafDER), b64(intDER), b64(rootDER)}
	jws := signJWS(t, leafKey, x5c, jwt.MapClaims{"transactionId": "TX1"})

	if err := verifyPayloadWithRoot(jws, rootPEM); err != nil {
		t.Fatalf("valid chain should verify, got: %v", err)
	}
}

// This is the regression guard for the x5c indexing bug: a payload whose leaf
// (x5c[0]) does NOT chain to the trusted root must be rejected. The old code
// verified x5c[2] (the root) against itself and wrongly passed.
func TestVerifyPayloadWithRoot_ForeignLeafRejected(t *testing.T) {
	rootPEM, _, _, intDER, rootDER := makeChain(t)

	// An unrelated, self-signed leaf that was NOT issued by the intermediate.
	foreignKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	foreignTmpl := &x509.Certificate{
		SerialNumber: big.NewInt(99),
		Subject:      pkix.Name{CommonName: "Foreign Leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	foreignDER, err := x509.CreateCertificate(rand.Reader, foreignTmpl, foreignTmpl, &foreignKey.PublicKey, foreignKey)
	if err != nil {
		t.Fatalf("foreign cert: %v", err)
	}
	x5c := []string{b64(foreignDER), b64(intDER), b64(rootDER)}
	jws := signJWS(t, foreignKey, x5c, jwt.MapClaims{"transactionId": "TX1"})

	if err := verifyPayloadWithRoot(jws, rootPEM); err == nil {
		t.Fatal("foreign leaf must be rejected, but verification passed")
	}
}

// verifyPayload (Apple production root) must reject our test chain, proving it
// actually exercises the leaf rather than trivially passing.
func TestVerifyPayload_RejectsNonAppleChain(t *testing.T) {
	_, leafKey, leafDER, intDER, rootDER := makeChain(t)
	x5c := []string{b64(leafDER), b64(intDER), b64(rootDER)}
	jws := signJWS(t, leafKey, x5c, jwt.MapClaims{"transactionId": "TX1"})

	if err := verifyPayload(jws); err == nil {
		t.Fatal("non-Apple chain must not verify against the embedded Apple root")
	}
}

// ---------- B: GetAllSubscriptionStatuses ----------

func TestBuildSubscriptionStatusURL(t *testing.T) {
	got := buildSubscriptionStatusURL("https://h", "TX1", nil)
	if got != "https://h/inApps/v1/subscriptions/TX1" {
		t.Fatalf("unexpected url: %s", got)
	}
	got = buildSubscriptionStatusURL("https://h", "TX1", []int32{SubscriptionStatus_Active, SubscriptionStatus_BillingRetry})
	if !strings.HasPrefix(got, "https://h/inApps/v1/subscriptions/TX1?") ||
		!strings.Contains(got, "status=1") || !strings.Contains(got, "status=3") {
		t.Fatalf("unexpected url with statuses: %s", got)
	}
}

func TestLastTransactionsItemDecode(t *testing.T) {
	_, leafKey, leafDER, intDER, rootDER := makeChain(t)
	x5c := []string{b64(leafDER), b64(intDER), b64(rootDER)}
	txJWS := signJWS(t, leafKey, x5c, jwt.MapClaims{
		"transactionId": "TX1",
		"productId":     "io.kaitu.sub.family.1y",
		"bundleId":      "io.kaitu.app",
		"expiresDate":   int64(4102444800000),
	})
	rnJWS := signJWS(t, leafKey, x5c, jwt.MapClaims{
		"originalTransactionId": "OTX1",
		"productId":             "io.kaitu.sub.family.1y",
		"autoRenewStatus":       int32(1),
	})
	it := &LastTransactionsItem{SignedTransactionInfo: txJWS, SignedRenewalInfo: rnJWS}

	ti, err := it.DecodeTransaction()
	if err != nil {
		t.Fatalf("decode transaction: %v", err)
	}
	if ti.ProductId != "io.kaitu.sub.family.1y" || ti.ExpiresDate != 4102444800000 {
		t.Fatalf("unexpected transaction info: %+v", ti)
	}
	ri, err := it.DecodeRenewal()
	if err != nil {
		t.Fatalf("decode renewal: %v", err)
	}
	if ri.AutoRenewStatus != 1 || ri.OriginalTransactionId != "OTX1" {
		t.Fatalf("unexpected renewal info: %+v", ri)
	}
}

func TestGetAllSubscriptionStatuses_ProdFailsSandboxSucceeds(t *testing.T) {
	setTestIapKey(t)

	sample := `{"environment":"Sandbox","bundleId":"io.kaitu.app","appAppleId":123,` +
		`"data":[{"subscriptionGroupIdentifier":"G1","lastTransactions":[` +
		`{"status":1,"originalTransactionId":"OTX1","signedTransactionInfo":"","signedRenewalInfo":""}]}]}`

	prod := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer prod.Close()

	var sandboxHit bool
	sandbox := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sandboxHit = true
		if !strings.Contains(r.URL.Path, "/inApps/v1/subscriptions/TX1") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sample))
	}))
	defer sandbox.Close()

	oldP, oldS := subStatusBaseProd, subStatusBaseSandbox
	subStatusBaseProd, subStatusBaseSandbox = prod.URL, sandbox.URL
	t.Cleanup(func() { subStatusBaseProd, subStatusBaseSandbox = oldP, oldS })

	resp, err := GetAllSubscriptionStatuses(context.Background(), "io.kaitu.app", "TX1")
	if err != nil {
		t.Fatalf("expected sandbox fallback to succeed: %v", err)
	}
	if !sandboxHit {
		t.Fatal("sandbox endpoint was never hit")
	}
	if len(resp.Data) != 1 || len(resp.Data[0].LastTransactions) != 1 {
		t.Fatalf("unexpected response shape: %+v", resp)
	}
	lt := resp.Data[0].LastTransactions[0]
	if lt.OriginalTransactionId != "OTX1" || lt.Status != SubscriptionStatus_Active {
		t.Fatalf("unexpected last transaction: %+v", lt)
	}
}

func TestGetAllSubscriptionStatuses_RequiresArgs(t *testing.T) {
	if _, err := GetAllSubscriptionStatuses(context.Background(), "", "TX1"); err == nil {
		t.Fatal("expected error for empty bundleId")
	}
	if _, err := GetAllSubscriptionStatuses(context.Background(), "io.kaitu.app", ""); err == nil {
		t.Fatal("expected error for empty transactionId")
	}
}
