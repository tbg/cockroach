// Copyright 2020 The Cockroach Authors.
//
// Use of this software is governed by the Business Source License
// included in the file licenses/BSL.txt.
//
// As of the Change Date specified in that file, in accordance with
// the Business Source License, use of this software will be governed
// by the Apache License, Version 2.0, included in the file
// licenses/APL.txt.

package metricdoctor

import (
	"io"
	"os"
	"strings"
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkGauge(m *dto.MetricFamily) {

}

func TestFoo(t *testing.T) {
	f, err := os.Open("var.txt")
	require.NoError(t, err)
	dec := expfmt.NewDecoder(f, expfmt.FmtText)

	for {
		var mf dto.MetricFamily
		err := dec.Decode(&mf)
		if err == io.EOF {
			break
		}
		require.NoError(t, err)
		if typ := mf.GetType(); typ != dto.MetricType_GAUGE {
			continue
		}
		if !strings.Contains(mf.GetName(), "slow") {
			continue
		}
		require.Len(t, mf.Metric, 1, mf.Name)
		v := mf.Metric[0].GetGauge().GetValue()
		assert.Zero(t, v, mf.GetName())
	}
}
