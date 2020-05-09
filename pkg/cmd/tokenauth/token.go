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
	"encoding/base64"
	"time"

	"github.com/gogo/protobuf/proto"
	"golang.org/x/crypto/nacl/sign"
)

// Mint makes a token for the provided id and expiration using the private key.
//
// TODO(tbg): consider base64 encoding.
func Mint(id uint64, expiration time.Time, privateKey *[64]byte) string {
	var message []byte
	message = append(message, proto.EncodeVarint(id)...)
	message = append(message, proto.EncodeVarint(uint64(expiration.UnixNano()))...)
	return base64.StdEncoding.EncodeToString(sign.Sign(nil, message, privateKey))
}

// Verify validates a token. This entails checking that it is well-formed,
// that its signature is confirmed via the provided public key, and that it is
// not expired as of the provided timestamp.
//
// On success, returns the token's ID, expiration time, and true.
// Otherwise, returns zero values.
func Verify(tok64 string, now time.Time, pubKey *[32]byte) (id uint64, _ time.Time, _ bool) {
	if len(tok64) > 10000 {
		// Can't be a real token, too long.
		return 0, time.Time{}, false
	}
	tok, err := base64.StdEncoding.DecodeString(tok64)
	if err != nil {
		return 0, time.Time{}, false
	}
	now = now.UTC()
	b, ok := sign.Open(nil, tok, pubKey)
	if !ok {
		return 0, time.Time{}, false
	}
	id, n := proto.DecodeVarint(b)
	if n == 0 {
		return 0, time.Time{}, false
	}
	b = b[n:]
	nanos, n := proto.DecodeVarint(b)
	if n != len(b) {
		return 0, time.Time{}, false
	}

	expiration := time.Unix(0, int64(nanos)).UTC()
	if expiration.Before(now) {
		return 0, time.Time{}, false
	}

	return id, expiration, true
}
