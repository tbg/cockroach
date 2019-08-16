package stateloader

import (
	"strconv"
	"strings"
	"testing"

	"github.com/cockroachdb/cockroach/pkg/roachpb"
	"github.com/cockroachdb/datadriven"
	"github.com/stretchr/testify/require"
	"go.etcd.io/etcd/raft"
)

var testMakeConfState = `
# Not in a joint config, just two voters.
confstate 1 2
----
Voters:[1 2] VotersOutgoing:[] Learners:[] LearnersNext:[] AutoLeave:false

# A voter and an outgoing voter.
confstate 1 2OUTGOING
----
Voters:[1] VotersOutgoing:[2] Learners:[] LearnersNext:[] AutoLeave:false

# Test that a learner in the incoming config gets read into LearnersNext if it
# overlaps an outgoing voter.
#
# TODO(tbg): demotions aren't implemented in CRDB yet. We need to add a new type
# ReplicaType_VOTERDEMOTING if we ever want that.
# cs 1 2LEARNER 2DEMOTING
# ----
# Voters:[1] VotersOutgoing:[2] Learners:[] LearnersNext:[2] AutoLeave:false

# Learner.
confstate 1 2LEARNER
----
Voters:[1] VotersOutgoing:[] Learners:[2] LearnersNext:[] AutoLeave:false
`

func TestMakeConfState(t *testing.T) {
	datadriven.RunTestFromString(t, testMakeConfState, func(d *datadriven.TestData) string {
		var reps []roachpb.ReplicaDescriptor
		for _, arg := range d.CmdArgs {
			tok := arg.Key
			var rep roachpb.ReplicaDescriptor
			rep.Type = roachpb.ReplicaTypeVoter()
			if strings.HasSuffix(tok, "LEARNER") {
				tok = tok[:len(tok)-7]
				rep.Type = roachpb.ReplicaTypeLearner()
			}
			if strings.HasSuffix(tok, "OUTGOING") {
				tok = tok[:len(tok)-8]
				rep.Type = roachpb.ReplicaTypeVoterOutgoing()
			}
			id, err := strconv.ParseUint(tok, 10, 32)
			require.NoError(t, err)
			rep.ReplicaID, rep.StoreID, rep.NodeID = roachpb.ReplicaID(id), roachpb.StoreID(id), roachpb.NodeID(id)
			reps = append(reps, rep)
		}
		return raft.DescribeConfState(makeConfState(reps)) + "\n"
	})
}
