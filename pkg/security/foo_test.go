// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/cmd/tokenauth"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/errors"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

func readFile(t *testing.T, path string) []byte {
	b, err := ioutil.ReadFile(path)
	require.NoError(t, err)
	return b
}

func TestFoo(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	td := "certs_test"
	require.NoError(t, os.RemoveAll(td))
	require.NoError(t, os.MkdirAll(td, 0700))
	defer os.RemoveAll(td)

	tdBroker := filepath.Join(td, "broker")
	certsBrokerKey := filepath.Join(tdBroker, "ca.key")
	require.NoError(t, CreateCAPair(
		tdBroker,
		certsBrokerKey,
		2048,
		2*time.Minute, // basically long-lived
		false,         // allowKeyReuse
		false,         // overwrite
	))
	certsBrokerCert := filepath.Join(tdBroker, "ca.crt")

	// Pretend we're the auth broker. We will make a certificate for a tenant which
	// that tenant should use as a client cert when auth'ing with the KV server.

	// First, make a public/private key pair. It will need to get signed by the CA.
	tenantKeyPair, err := rsa.GenerateKey(rand.Reader, 2048)
	tenantKeyPairFile := filepath.Join(td, "tenant.key")
	require.NoError(t, writeKeyToFile(tenantKeyPairFile, tenantKeyPair, false))
	brokerCert, brokerKey, err := loadCACertAndKey(certsBrokerCert, certsBrokerKey)
	require.NoError(t, err)

	// Set up various options.
	template, err := generateServerCertTemplate(
		brokerCert,
		time.Minute,
		"tenant195",
		[]string{"127.0.0.1"},
	)
	// The cert will only allow client auth.
	template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	require.NoError(t, err)

	t.Logf("%+v", template)

	// The broker CA creates a certificate.
	tenantCertBytes, err := x509.CreateCertificate(
		rand.Reader, template, brokerCert, tenantKeyPair.Public(), brokerKey)
	require.NoError(t, err)
	tenantCertFile := filepath.Join(td, "tenant.crt")
	require.NoError(t, writeCertificateToFile(tenantCertFile, tenantCertBytes, false))

	// End of pretending to be the auth broker.

	// newServerTLSConfig creates a server TLSConfig from the supplied byte strings containing
	// - the certificate of this node (should be signed by the CA),
	// - the private key of this node.
	// - the certificate of the cluster CA, used to verify other server certificates
	// - the certificate of the client CA, used to verify client certificates
	kvCert := readFile(t, "securitytest/test_certs/node.crt")
	kvCertKey := readFile(t, "securitytest/test_certs/node.key")
	serverTLSConfig, err := newServerTLSConfig(
		kvCert,
		kvCertKey,
		nil,
		readFile(t, certsBrokerCert), // broker is the "client CA"
	)
	require.NoError(t, err)

	// - the certificate of this client (should be signed by the CA),
	// - the private key of this client.
	// - the certificate of the cluster CA (use system cert pool if nil)
	clientTLSConfig, err := newClientTLSConfig(
		readFile(t, tenantCertFile),
		readFile(t, tenantKeyPairFile),
		readFile(t, "securitytest/test_certs/ca.crt"), // to validate KV server
	)
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	ic := func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		peer, ok := peer.FromContext(ctx)
		if !ok {
			return nil, errors.New("no peer info found")
		}

		tlsInfo, ok := peer.AuthInfo.(credentials.TLSInfo)
		if !ok {
			return nil, errors.New("no TLSInfo found")
		}

		t.Logf("RPC from %s, valid until %s\n", tlsInfo.State.PeerCertificates[0].Subject.CommonName, tlsInfo.State.PeerCertificates[0].NotAfter)

		return handler(ctx, req)
	}
	kvGRPC := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(serverTLSConfig)),
		grpc.UnaryInterceptor(ic),
	)
	defer kvGRPC.Stop()
	tokenauth.RegisterMockKVServer(kvGRPC, &mockKVServer{})
	go func() {
		if err := kvGRPC.Serve(ln); err != nil {
			panic(err)
		}
	}()

	addr := ln.Addr().String()
	cc, err := grpc.Dial(addr, grpc.WithTransportCredentials(credentials.NewTLS(clientTLSConfig)))
	require.NoError(t, err)
	defer cc.Close()
	_, err = tokenauth.NewMockKVClient(cc).Get(ctx, &tokenauth.GetRequest{})
	require.NoError(t, err)

	return
}

type mockKVServer struct {
}

func (s *mockKVServer) Get(
	ctx context.Context, req *tokenauth.GetRequest,
) (*tokenauth.GetResponse, error) {
	return &tokenauth.GetResponse{}, nil
}
