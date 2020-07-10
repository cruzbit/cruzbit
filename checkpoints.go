// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"fmt"
)

// CheckpointsEnabled can be disabled for testing.
const CheckpointsEnabled = true

// LatestCheckpointHeight is used to determine if the client is synced.
const LatestCheckpointHeight = 145142

// Checkpoints are known height and block ID pairs on the main chain.
var Checkpoints map[int64]string = map[int64]string{
	18144:  "000000000000b83e78ec29355d098256936389010d7450a288763ed4f191069e",
	36288:  "00000000000052bd43e85cf60f2ecd1c5016083e6a560b3ee57427c7f2dd64e8",
	54432:  "000000000001131d0597533737d7aadac0a5d4e132caa4c47c793c02e6d56063",
	72576:  "0000000000013873c9974f8468c7e03419e02f49aaf9761f4d6c19e233d0bb3d",
	90720:  "0000000000026254d69f914ff774ed8691d30003c8094d03e61aa8ed4c862c5f",
	108864: "00000000001d7b35c09ac85a4e5b577dc62569f2782220723f1613ea268c66aa",
	127008: "000000000013df027075d395d6f97e03cd8285db6c37b1575e66ede1c480d3de",
	145142: "0000000006dcd69479a3f4f40a301d22e78b1f56de44e00c1fa3191967fd1425",
}

// CheckpointCheck returns an error if the passed height is a checkpoint and the
// passed block ID does not match the given checkpoint block ID.
func CheckpointCheck(id BlockID, height int64) error {
	if !CheckpointsEnabled {
		return nil
	}
	checkpointID, ok := Checkpoints[height]
	if !ok {
		return nil
	}
	if id.String() != checkpointID {
		return fmt.Errorf("Block %s at height %d does not match checkpoint ID %s",
			id, height, checkpointID)
	}
	return nil
}
