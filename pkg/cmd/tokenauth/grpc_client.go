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
	context "context"

	"google.golang.org/grpc/credentials"
)

type clientTokenCreds struct {
	m map[string]string
}

func (c *clientTokenCreds) GetRequestMetadata(
	context.Context, ...string,
) (map[string]string, error) {
	return c.m, nil
}

func (c *clientTokenCreds) RequireTransportSecurity() bool {
	return true
}

func PerRPCCredentials(token string) credentials.PerRPCCredentials {
	return &clientTokenCreds{m: map[string]string{tokenKey: token}}
}
