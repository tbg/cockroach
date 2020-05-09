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
	context "context"
	"time"

	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	tokenKey = "X-CRL-KV"

	msgTokenMissing = "missing token"
	msgTokenInvalid = "invalid token"
)

func NewServer(tm TokenAuthMap, opts ...grpc.ServerOption) *grpc.Server {
	o := append([]grpc.ServerOption{
		grpc.UnaryInterceptor(tm.m.UnaryServerInterceptor),
		grpc.StreamInterceptor(tm.m.StreamServerInterceptor),
	}, opts...)
	return grpc.NewServer(o...)
}

type AuthN struct {
	id         uint64
	now        func() time.Time
	expiration time.Time
}

func (a *AuthN) ID() uint64 {
	return a.id
}

func (a *AuthN) Valid() bool {
	return a.expiration.After(a.now())
}

func (a *AuthN) Expiration() time.Time {
	return a.expiration
}

type TokenAuthMap struct {
	now       func() time.Time
	publicKey *[32]byte

	m MethodAuthMap
}

func MakeTokenAuthMap(now func() time.Time, publicKey *[32]byte) TokenAuthMap {
	return TokenAuthMap{
		now:       now,
		publicKey: publicKey,
	}
}

type TokenAuthHook struct {
	AuthZ func(ctx context.Context, authN AuthN, req interface{}) error
	Post  func(ctx context.Context, authN AuthN, req interface{}) error
}

func (tm *TokenAuthMap) Add(fullMethod string, hook TokenAuthHook) {
	if tm.m == nil {
		tm.m = MethodAuthMap{}
	}
	if _, ok := tm.m[fullMethod]; ok {
		panic("double registration: " + fullMethod)
	}
	tm.m[fullMethod] = AuthHook{
		AuthN: func(ctx context.Context) (context.Context, error) {
			authN, err := verifyTokenFromGRPCMeta(ctx, tm.now, tm.publicKey)
			if err != nil {
				return nil, err
			}
			return ctxWithAuthN(ctx, &authN), nil
		},
		AuthZ: func(ctx context.Context, req interface{}) error {
			authN, ok := FromCtx(ctx)
			if !ok {
				return status.Error(codes.PermissionDenied, "")
			}
			return hook.AuthZ(ctx, *authN, req)
		},
		Post: func(ctx context.Context, resp interface{}) error {
			authN, ok := FromCtx(ctx)
			if !ok {
				return status.Error(codes.PermissionDenied, "")
			}
			return hook.Post(ctx, *authN, resp)
		},
	}
}

func verifyTokenFromGRPCMeta(
	ctx context.Context, now func() time.Time, publicKey *[32]byte,
) (AuthN, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return AuthN{}, status.Error(codes.Unauthenticated, msgTokenMissing)
	}
	vs := md.Get(tokenKey)
	if len(vs) != 1 {
		return AuthN{}, status.Error(codes.Unauthenticated, msgTokenMissing)
	}

	id, expiration, ok := Verify(vs[0], now(), publicKey)
	if !ok {
		return AuthN{}, status.Error(codes.Unauthenticated, msgTokenInvalid)
	}
	return AuthN{
		id:         id,
		expiration: expiration,
		now:        now,
	}, nil
}
