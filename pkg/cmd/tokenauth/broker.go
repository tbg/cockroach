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
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Broker struct {
	publicKey  *[32]byte
	privateKey *[64]byte
}

func (ab *Broker) MakeToken(id uint64, expiration time.Time) string {
	return Mint(id, expiration, ab.privateKey)
}

const extension = 10 * time.Second

func (ab *Broker) RefreshToken(authN *AuthN) (string, error) {
	if !authN.Valid() {
		return "", status.Error(codes.Unauthenticated, "")
	}
	return ab.MakeToken(authN.ID(), authN.Expiration().Add(extension)), nil
}
