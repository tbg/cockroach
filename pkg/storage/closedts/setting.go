// Copyright 2018 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package closedts

import (
	"errors"
	"time"

	"github.com/cockroachdb/cockroach/pkg/settings"
)

// TargetDuration is the follower reads closed timestamp update target duration.
var TargetDuration = settings.RegisterNonNegativeDurationSetting(
	"kv.closed_timestamp.target_duration",
	"if nonzero, attempt to provide closed timestamp notifications for timestamps trailing "+
		"cluster time by approximately this duration, backing off as necessary up to a factor "+
		"of kv.closed_timestamp.max_backoff_multiple to avoid starving long-running transactions",
	time.Second,
)

// MaxBackoffMultiple multiplied by TargetDuration is the longest transaction
// duration that will be respected by closed timestamps. Whenever closing out a
// timestamp fails due to a still-ongoing tracked operation, the next Close will
// be delayed more (unless the multiple is already that specified above).
// Whenever closing out works without an intermittent forced noop-close, the
// effective target duration becomes more aggressive.
//
// The effect of this mechanism is roughly that the closed timestamp period
// hovers around the duration of the longest transaction (or the maximum
// duration tolerated, whichever is smaller), which most importantly allows
// any long-running transaction to finish if it is prepared to retry.
var MaxBackoffMultiple = settings.RegisterValidatedIntSetting(
	"kv.closed_timestamp.max_backoff_multiple",
	"the maximum factor by which to delay closed timestamps (applied against "+
		"kv.closed_timestamp.target_duration) in order to allow long-running transactions "+
		"to finish",
	30,
	func(i int64) error {
		if i < 1 {
			return errors.New("backoff multipler must be positive")
		}
		return nil
	},
)

// CloseFraction is the fraction of TargetDuration determining how often closed
// timestamp updates are to be attempted.
var CloseFraction = settings.RegisterValidatedFloatSetting(
	"kv.closed_timestamp.close_fraction",
	"fraction of closed timestamp target duration specifying how frequently the closed timestamp is advanced",
	0.2,
	func(v float64) error {
		if v <= 0 || v > 1 {
			return errors.New("value not between zero and one")
		}
		return nil
	})
