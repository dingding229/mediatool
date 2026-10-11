package clouddrive2

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	pb "media-tool/internal/adapters/clouddrive2/proto"
	"media-tool/internal/modules/storage"
)

func TestTLSRequiresTrustedCertificateAndAcceptsConfiguredCA(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "CloudDrive2 protocol fixture"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), BasicConstraintsValid: true, IsCA: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	server := grpc.NewServer(grpc.Creds(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}})))
	pb.RegisterCloudDriveFileSrvServer(server, &protocolServer{currentToken: "token", fileType: pb.CloudDriveFile_File})
	go server.Serve(listener)
	defer server.Stop()
	opts := Options{Endpoint: "https://" + listener.Addr().String(), Token: "token", RequestTimeout: 200 * time.Millisecond}
	untrusted, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer untrusted.Close()
	if _, err := untrusted.Find(context.Background(), "/115/Media/a.mkv"); !errors.Is(err, storage.ErrUnavailable) {
		t.Fatal("untrusted certificate accepted", err)
	}
	ca := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	opts.CAFile = ca
	opts.RequestTimeout = time.Second
	trusted, err := New(opts)
	if err != nil {
		t.Fatal(err)
	}
	defer trusted.Close()
	if _, err := trusted.Find(context.Background(), "/115/Media/a.mkv"); err != nil {
		t.Fatal("configured CA rejected", err)
	}
}
