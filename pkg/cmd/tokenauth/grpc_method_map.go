// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package tokenauth

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type AuthHook struct {
	AuthN func(context.Context) (context.Context, error)
	AuthZ func(context.Context, interface{}) error
	Post  func(context.Context, interface{}) error
}

type MethodAuthMap map[string]AuthHook

func (m MethodAuthMap) UnaryServerInterceptor(
	ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler,
) (interface{}, error) {
	hook, ok := m[info.FullMethod]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "")
	}
	ctx, err := hook.AuthN(ctx)
	if err != nil {
		return nil, err
	}
	if err := hook.AuthZ(ctx, req); err != nil {
		return nil, err
	}

	resp, err := handler(ctx, req)
	if err != nil {
		return nil, err
	}

	if err := hook.Post(ctx, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

func (m MethodAuthMap) StreamServerInterceptor(
	srv interface{},
	stream grpc.ServerStream,
	info *grpc.StreamServerInfo,
	handler grpc.StreamHandler,
) error {
	hook, ok := m[info.FullMethod]
	if !ok {
		return status.Error(codes.Unauthenticated, "")
	}
	ctx := stream.Context()
	ctx, err := hook.AuthN(ctx)
	if err != nil {
		return err
	}
	wrapped := &wrappedServerStream{
		ServerStream: stream,
		ctx:          ctx,
		hook:         hook,
		fullMethod:   info.FullMethod,
	}
	return handler(srv, wrapped)
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx        context.Context
	hook       AuthHook
	fullMethod string
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

func (w *wrappedServerStream) RecvMsg(req interface{}) error {
	if err := w.hook.AuthZ(w.ctx, req); err != nil {
		return err
	}
	return w.ServerStream.RecvMsg(req)
}

func (w *wrappedServerStream) SendMsg(resp interface{}) error {
	if err := w.hook.Post(w.ctx, resp); err != nil {
		return err
	}
	return w.ServerStream.SendMsg(resp)
}
