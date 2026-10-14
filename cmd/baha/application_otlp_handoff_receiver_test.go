package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/mcpdev80/baseharbor/internal/externalprovider"
	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

type nativeOTLPReceiver struct {
	host, project, network, compose, env string
	registration                         externalprovider.Registration
	material                             map[string][32]byte
}

func (r nativeOTLPReceiver) waitReady(t *testing.T, parent context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := externalprovider.Verify(ctx, r.registration); err == nil {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("independent native receiver did not establish verified mTLS readiness")
		case <-ticker.C:
		}
	}
}

func (r nativeOTLPReceiver) assertMaterials(t *testing.T) {
	t.Helper()
	assertNativeOTLPMaterialSnapshot(t, r.material)
}

func (r nativeOTLPReceiver) assertWorkload(t *testing.T, ctx context.Context, runtime bhruntime.RuntimeProvider, actor, payload string) {
	t.Helper()
	out, err := runtime.ExecProject(ctx, r.project, r.compose, r.env, "receiver", "cat", "/tmp/receipts.json")
	if err != nil {
		t.Fatal("independent receiver receipt unavailable", err)
	}
	var receipts []struct {
		Actor, Client string
		Size          int
		Digest        string
	}
	if err := json.Unmarshal([]byte(out), &receipts); err != nil {
		t.Fatal("receiver receipt invalid", err)
	}
	body, err := hex.DecodeString(payload)
	if err != nil {
		t.Fatal("invalid fixture Protobuf payload", err)
	}
	digest := sha256.Sum256(body)
	for _, receipt := range receipts {
		if receipt.Actor == actor && receipt.Client == "native-workload-client" && receipt.Size == len(body) && receipt.Digest == hex.EncodeToString(digest[:]) {
			return
		}
	}
	t.Fatal("receiver did not verify the actual native workload mTLS/credential/Protobuf request")
}

func TestNativeOTLPFixtureDNSResolvesWithStandardResolver(t *testing.T) {
	const host = "receiver.fixture.test"
	installNativeOTLPFixtureDNS(t, host)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil || len(addresses) != 1 || addresses[0] != "127.0.0.1" {
		t.Fatalf("native fixture DNS did not resolve through normal Go EDNS/A/AAAA queries: %v %v", addresses, err)
	}
}

// The fixture host reaches the published native port; the workload reaches the
// independent receiver's native network alias. Both resolve the same hostname
// and verify the same real server certificate without a transport override.
func installNativeOTLPFixtureDNS(t *testing.T, host string) {
	t.Helper()
	listener, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "udp", listener.LocalAddr().String())
	}}
	t.Cleanup(func() { net.DefaultResolver = original; _ = listener.Close() })
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, address, err := listener.ReadFrom(buffer)
			if err != nil {
				return
			}
			encoded := nativeOTLPDNSResponse(buffer[:n], host)
			if encoded != nil {
				_, _ = listener.WriteTo(encoded, address)
			}
		}
	}()
}

// Answer only the fixture's single A/AAAA question. This bounded DNS fixture
// has no forwarding, compressed-question parser or external dependency.
func nativeOTLPDNSResponse(query []byte, host string) []byte {
	if len(query) < 17 || binary.BigEndian.Uint16(query[4:6]) != 1 {
		return nil
	}
	offset := 12
	var labels []string
	for {
		if offset >= len(query) {
			return nil
		}
		length := int(query[offset])
		offset++
		if length == 0 {
			break
		}
		if length > 63 || offset+length > len(query) {
			return nil
		}
		labels = append(labels, string(query[offset:offset+length]))
		offset += length
	}
	if offset+4 > len(query) || !strings.EqualFold(strings.Join(labels, "."), host) {
		return nil
	}
	// Resolver queries may carry EDNS options after the single question.
	// Return only the question; no extra records are advertised in this reply.
	answer := append([]byte(nil), query[:offset+4]...)
	binary.BigEndian.PutUint16(answer[2:4], 0x8400|(binary.BigEndian.Uint16(query[2:4])&0x0100))
	for i := 6; i < 12; i++ {
		answer[i] = 0
	}
	if binary.BigEndian.Uint16(query[offset:offset+2]) == 1 && binary.BigEndian.Uint16(query[offset+2:offset+4]) == 1 {
		binary.BigEndian.PutUint16(answer[6:8], 1)
		answer = append(answer, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 0, 0, 4, 127, 0, 0, 1)
	}
	return answer
}

const nativeOTLPReceiverPython = `import hashlib,json,os,ssl,sys,threading
from http.server import BaseHTTPRequestHandler,ThreadingHTTPServer
receipts=[]
lock=threading.Lock()
class Receiver(BaseHTTPRequestHandler):
 def do_POST(self):
  length=int(self.headers.get('Content-Length','0'))
  if length<=0 or length>1048576:
   self.send_error(400);return
  body=self.rfile.read(length)
  with open('/credential.json') as f: token=json.load(f)['token']
  client=self.connection.getpeercert()
  subject=dict(pair for group in client['subject'] for pair in group)
  if self.path!='/v1/traces' or self.headers.get('Content-Type')!='application/x-protobuf' or self.headers.get('Authorization')!='Bearer '+token or subject.get('commonName')!='native-workload-client' or b'baseharbor.otlp.verify' not in body:
   self.send_error(401);return
  with lock:
   receipts.append({'Actor':self.headers.get('X-Fixture-Workload','host-verifier'),'Client':subject['commonName'],'Size':len(body),'Digest':hashlib.sha256(body).hexdigest()})
   with open('/tmp/receipts.next','w') as f:json.dump(receipts,f)
   os.replace('/tmp/receipts.next','/tmp/receipts.json')
  self.send_response(200);self.send_header('Content-Length','0');self.end_headers()
 def log_message(self,*args):pass
server=ThreadingHTTPServer(('0.0.0.0',int(sys.argv[1])),Receiver)
context=ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
context.minimum_version=ssl.TLSVersion.TLSv1_2
context.load_cert_chain('/server.pem','/server-key.pem')
context.load_verify_locations('/ca.pem')
context.verify_mode=ssl.CERT_REQUIRED
server.socket=context.wrap_socket(server.socket,server_side=True)
server.serve_forever()
`
