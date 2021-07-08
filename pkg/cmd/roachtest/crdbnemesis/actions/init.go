// Copyright 2021 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package actions

import "github.com/cockroachdb/cockroach/pkg/cmd/roachtest/crdbnemesis"

var all []crdbnemesis.ActionFactory

// All returns all ActionFactories in this package.
func All() []crdbnemesis.ActionFactory {
	return all
}
