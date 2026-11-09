package onboardingbundle

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"math/big"
	"strings"
	"time"

	ws "github.com/mcpdev80/baseharbor/internal/workstationenrollment"
)

// EnrolledIdentity is sensitive, verified material. The caller must persist it
// atomically with owner-only permissions before reporting onboarding success.
// Never serialize or print this struct in terminal output, logs or a portable
// invitation.
type EnrolledIdentity struct {
	PrivateKeyPEM []byte
	CertificatePEM []byte
	TrustPEM []byte
	InstallationID string
	Station string
	Serial string
	ExpiresAt time.Time
}

type enrollReply struct {
	InstallationID string `json:"installation_id"`
	Station string `json:"station"`
	Serial string `json:"serial"`
	LeafPEM string `json:"leaf_pem"`
	TrustPEM string `json:"trust_pem"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Redeem executes only the one-use enrollment exchange using a grant minted
// on the owning Core. It never asks for operator passwords or calls bootstrap;
// the initial grant must already exist in the durable Core authorization store.
// A failed verification returns no identity.
func Redeem(ctx context.Context, b Bundle, now time.Time) (EnrolledIdentity, error) {
	var zero EnrolledIdentity
	if err := b.Validate(now); err != nil { return zero, err }
	if err := ctx.Err(); err != nil { return zero, err }
	scope := ws.Scope{TenantID:b.InstallationID, Station:b.Station}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM([]byte(b.CAPEM)) {
		return zero, ErrInvalid
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil { return zero, err }
	uri, err := url.Parse(scope.Identity())
	if err != nil { return zero, ErrInvalid }
	csrDER, err := x509.CreateCertificateRequest(rand.Reader,
		&x509.CertificateRequest{URIs:[]*url.URL{uri}}, key)
	if err != nil { return zero, err }
	csr := pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE REQUEST",Bytes:csrDER})
	payload, err := json.Marshal(map[string]string{
		"installation_id":b.InstallationID,
		"station":b.Station,
		"token":b.Token,
		"nonce":b.Nonce,
		"csr_pem":string(csr),
	})
	if err != nil { return zero, err }
	req, err := http.NewRequestWithContext(ctx,http.MethodPost,b.CoreURL+"/api/v1/workstations/enroll",bytes.NewReader(payload))
	if err != nil { return zero, err }
	req.Header.Set("Content-Type","application/json")
	transport := &http.Transport{TLSClientConfig:&tls.Config{MinVersion:tls.VersionTLS12,RootCAs:roots}}
	defer transport.CloseIdleConnections()
	client:=&http.Client{Transport:transport,Timeout:20*time.Second}
	resp,err:=client.Do(req)
	if err != nil {return zero,fmt.Errorf("%w: Core TLS enrollment failed: %v",ErrInvalid,err)}
	defer resp.Body.Close()
	if resp.StatusCode!=http.StatusOK {
		return zero,fmt.Errorf("%w: Core rejected one-use invitation (HTTP %d)",ErrInvalid,resp.StatusCode)
	}
	var reply enrollReply
	dec:=json.NewDecoder(io.LimitReader(resp.Body,64<<10))
	dec.DisallowUnknownFields()
	if err:=dec.Decode(&reply);err!=nil{return zero,fmt.Errorf("%w: invalid enrollment response",ErrInvalid)}
	var extra any
	if err:=dec.Decode(&extra);err!=io.EOF{return zero,fmt.Errorf("%w: trailing enrollment response",ErrInvalid)}
	if reply.InstallationID!=b.InstallationID || reply.Station!=b.Station ||
		reply.Serial=="" || !reply.ExpiresAt.After(now) || reply.ExpiresAt.After(now.Add(24*time.Hour)) {
		return zero,fmt.Errorf("%w: Core enrollment identity mismatch",ErrInvalid)
	}
	block,rest:=pem.Decode([]byte(reply.LeafPEM))
	if block==nil || block.Type!="CERTIFICATE" || len(bytes.TrimSpace(rest))!=0 {
		return zero,fmt.Errorf("%w: malformed Core leaf certificate",ErrInvalid)
	}
	leaf,err:=x509.ParseCertificate(block.Bytes)
	if err!=nil{return zero,ErrInvalid}
	if _,err:=leaf.Verify(x509.VerifyOptions{Roots:roots,CurrentTime:now,KeyUsages:[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}});err!=nil {
		return zero,fmt.Errorf("%w: Core leaf chain invalid",ErrInvalid)
	}
	// Managed OpenBao and the local test issuer both report hex serials; the
	// former may separate octets with colons. Never compare decimal and hex
	// textual representations of the same certificate serial.
	serialText := strings.ReplaceAll(strings.ToLower(reply.Serial), ":", "")
	reportedSerial, ok := new(big.Int).SetString(serialText, 16)
	if !ok || reportedSerial.Cmp(leaf.SerialNumber)!=0 ||
		len(leaf.URIs)!=1 || leaf.URIs[0].String()!=scope.Identity() ||
		leaf.IsCA || leaf.NotAfter.Sub(reply.ExpiresAt) > time.Second ||
		reply.ExpiresAt.Sub(leaf.NotAfter) > time.Second {
		return zero,fmt.Errorf("%w: Core leaf identity not bound to invitation",ErrInvalid)
	}
	publicExpected,err:=x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err!=nil{return zero,err}
	publicActual,err:=x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err!=nil || !bytes.Equal(publicExpected,publicActual){
		return zero,fmt.Errorf("%w: Core returned another device's key",ErrInvalid)
	}
	if reply.TrustPEM=="" || !roots.AppendCertsFromPEM([]byte(reply.TrustPEM)) {
		return zero,fmt.Errorf("%w: Core omitted authoritative trust",ErrInvalid)
	}
	privateDER,err:=x509.MarshalECPrivateKey(key)
	if err!=nil{return zero,err}
	identity:=EnrolledIdentity{
		PrivateKeyPEM:pem.EncodeToMemory(&pem.Block{Type:"EC PRIVATE KEY",Bytes:privateDER}),
		CertificatePEM:[]byte(reply.LeafPEM),
		TrustPEM:[]byte(b.CAPEM),
		InstallationID:b.InstallationID,
		Station:b.Station,
		Serial:reply.Serial,
		ExpiresAt:reply.ExpiresAt,
	}
	if len(identity.PrivateKeyPEM)==0 || len(identity.CertificatePEM)==0 {
		return zero,errors.New("workstation identity material is unavailable")
	}
	return identity,nil
}
