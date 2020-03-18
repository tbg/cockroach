// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package sqlbase

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/clusterversion"
	"github.com/cockroachdb/cockroach/pkg/config/zonepb"
	"github.com/cockroachdb/cockroach/pkg/keys"
	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/cockroach/pkg/util/leaktest"
	"github.com/cockroachdb/datadriven"
)

func TestTenantInitialValues(t *testing.T) {
	defer leaktest.AfterTest(t)()
	tenantID := uint64(1)
	schema := MakeMetadataSchema(zonepb.DefaultZoneConfigRef(), zonepb.DefaultSystemZoneConfigRef(), tenantID)
	// TODO(tbg): hard-code current binary version.
	kvs, splits := schema.GetInitialValues(clusterversion.ClusterVersion{Version: roachpb.Version{Major: 9999}})
	_ = splits

	datadriven.RunTest(t, filepath.Join("testdata", "tenant_bootstrap"), func(t *testing.T, td *datadriven.TestData) string {
		var buf strings.Builder
		for _, kv := range kvs {
			fmt.Fprintln(&buf, keys.PrettyPrint(nil, kv.Key))
		}
		t.Log(buf.String())
		return buf.String()
	})
}
