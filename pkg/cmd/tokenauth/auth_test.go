// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/cockroach/pkg/util/stop"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/nacl/sign"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
)

const (
	certFile = "testserver.rsa.crt"
	keyFile  = "testserver.rsa.key"

	blacklistedID = 123
)

type mockKVServer struct {
	stopper            *stop.Stopper
	addr               string
	numSuccessfulAuthN int64
}

func (s *mockKVServer) Stopper() *stop.Stopper {
	return s.stopper
}

func (s *mockKVServer) Addr() string {
	return s.addr
}

func (s *mockKVServer) Get(ctx context.Context, req *GetRequest) (*GetResponse, error) {
	return &GetResponse{}, nil
}

func (s *mockKVServer) NumSuccessfulAuth() int64 {
	return s.numSuccessfulAuthN
}

func startMockKVServer(ctx context.Context, publicKey *[32]byte) (*mockKVServer, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, err
	}

	srv := &mockKVServer{stopper: stop.NewStopper(), addr: ln.Addr().String()}

	tm := MakeTokenAuthMap(time.Now, publicKey)
	tm.Add("/"+_MockKV_serviceDesc.ServiceName+"/Get", TokenAuthHook{
		AuthZ: func(ctx context.Context, authN AuthN, req interface{}) error {
			srv.numSuccessfulAuthN++
			if authN.ID() == blacklistedID {
				return status.Error(codes.PermissionDenied, "blacklisted")
			}
			return nil
		},
		Post: func(ctx context.Context, authN AuthN, resp interface{}) error {
			_, err := fmt.Printf("id=%d <- %+v\n", authN.ID(), resp.(*GetResponse))
			return err
		},
	})

	tlsCreds, err := credentials.NewServerTLSFromFile(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	s := NewServer(tm, grpc.Creds(tlsCreds))
	RegisterMockKVServer(s, srv)
	srv.stopper.RunWorker(ctx, func(context.Context) {
		s.Serve(ln)
	})
	srv.stopper.RunWorker(ctx, func(context.Context) {
		<-srv.stopper.ShouldStop()
		s.Stop()
	})
	return srv, nil
}

func TestToken(t *testing.T) {
	defer leaktest.AfterTest(t)()
	ctx := context.Background()

	publicKey, privateKey, err := sign.GenerateKey(rand.Reader)
	require.NoError(t, err)

	now := time.Now

	validToken := Mint(12, now().Add(24*time.Hour), privateKey)
	blacklistedToken := Mint(blacklistedID, now().Add(24*time.Hour), privateKey)

	s, err := startMockKVServer(ctx, publicKey)
	require.NoError(t, err)
	defer s.Stopper().Stop(ctx)

	t.Run("fail-authN", func(t *testing.T) {
		c, err := newMockKVClient(s.Stopper(), s.Addr(), "wrong-token")
		require.NoError(t, err)
		_, err = c.Get(ctx, &GetRequest{})
		require.Error(t, err)
		require.Equal(t, codes.Unauthenticated, status.Code(err))
		require.EqualValues(t, 0, s.NumSuccessfulAuth())
	})

	t.Run("success", func(t *testing.T) {
		t.Run("unary", func(t *testing.T) {
			c, err := newMockKVClient(s.Stopper(), s.Addr(), validToken)
			require.NoError(t, err)
			_, err = c.Get(ctx, &GetRequest{})
			require.NoError(t, err)
			// TODO(tbg): this is too stateful between subtests.
			require.EqualValues(t, 1, s.NumSuccessfulAuth())
		})
		// TODO stream
	})

	t.Run("fail-authZ", func(t *testing.T) {
		c, err := newMockKVClient(s.Stopper(), s.Addr(), blacklistedToken)
		require.NoError(t, err)
		_, err = c.Get(ctx, &GetRequest{})
		require.Error(t, err)
		require.Equal(t, codes.PermissionDenied, status.Code(err))
	})
}

func newMockKVClient(stopper *stop.Stopper, addr string, token string) (MockKVClient, error) {
	creds, err := credentials.NewClientTLSFromFile(certFile, "localhost")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.Dial(
		addr,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(PerRPCCredentials(token)),
	)
	if err != nil {
		return nil, err
	}
	stopper.AddCloser(stop.CloserFn(func() { _ = conn.Close() }))
	return NewMockKVClient(conn), nil
}
