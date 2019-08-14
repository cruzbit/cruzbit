// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import (
	"fmt"
)

// CheckpointsEnabled can be disabled for testing.
const CheckpointsEnabled = true

// LatestCheckpointHeight is used to determine if the client is synced.
const LatestCheckpointHeight = 18144

// Checkpoints are known height and block ID pairs on the main chain.
var Checkpoints map[int64]string = map[int64]string{
	18144: "000000000000b83e78ec29355d098256936389010d7450a288763ed4f191069e",
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
