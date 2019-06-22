// Copyright 2019 cruzbit developers
// Use of this source code is governed by a MIT-style license that can be found in the LICENSE file.

package cruzbit

import "testing"

func TestBlockCreationReward(t *testing.T) {
	var maxHalvings int64 = 64
	var previous int64 = INITIAL_COINBASE_REWARD * 2
	var halvings int64
	for halvings = 0; halvings < maxHalvings; halvings++ {
		var height int64 = halvings * BLOCKS_UNTIL_REWARD_HALVING
		reward := BlockCreationReward(height)
		if reward > INITIAL_COINBASE_REWARD {
			t.Fatalf("Reward %d at height %d greater than initial reward %d",
				reward, height, INITIAL_COINBASE_REWARD)
		}
		if reward != previous/2 {
			t.Fatalf("Reward %d at height %d not equal to half previous period reward",
				reward, height)
		}
		previous = reward
	}
	if BlockCreationReward(maxHalvings*BLOCKS_UNTIL_REWARD_HALVING) != 0 {
		t.Fatalf("Expected 0 reward by %d halving", maxHalvings)
	}
}

func TestComputeMaxTransactionsPerBlock(t *testing.T) {
	var maxDoublings int64 = 64
	var doublings int64
	previous := INITIAL_MAX_TRANSACTIONS_PER_BLOCK / 2
	// verify the max is always doubling as expected
	for doublings = 0; doublings < maxDoublings; doublings++ {
		var height int64 = doublings * BLOCKS_UNTIL_TRANSACTIONS_PER_BLOCK_DOUBLING
		max := computeMaxTransactionsPerBlock(height)
		if max < INITIAL_MAX_TRANSACTIONS_PER_BLOCK {
			t.Fatalf("Max %d at height %d less than initial", max, height)
		}
		expect := previous * 2
		if expect > MAX_TRANSACTIONS_PER_BLOCK {
			expect = MAX_TRANSACTIONS_PER_BLOCK
		}
		if max != expect {
			t.Fatalf("Max %d at height %d not equal to expected max %d",
				max, height, expect)
		}
		if doublings > 0 {
			var previous2 int = max
			// walk back over the previous period and make sure:
			// 1) the max is never greater than this period's first max
			// 2) the max is always <= the previous as we walk back
			for height -= 1; height >= (doublings-1)*BLOCKS_UNTIL_TRANSACTIONS_PER_BLOCK_DOUBLING; height-- {
				max2 := computeMaxTransactionsPerBlock(height)
				if max2 > max {
					t.Fatalf("Max %d at height %d is greater than next period's first max %d",
						max2, height, max)
				}
				if max2 > previous2 {
					t.Fatalf("Max %d at height %d is greater than previous max %d at height %d",
						max2, height, previous2, height+1)
				}
				previous2 = max2
			}
		}
		previous = max
	}
	max := computeMaxTransactionsPerBlock(MAX_TRANSACTIONS_PER_BLOCK_EXCEEDED_AT_HEIGHT)
	if max != MAX_TRANSACTIONS_PER_BLOCK {
		t.Fatalf("Expected %d at height %d, found %d",
			MAX_TRANSACTIONS_PER_BLOCK, MAX_TRANSACTIONS_PER_BLOCK_EXCEEDED_AT_HEIGHT, max)
	}
	max = computeMaxTransactionsPerBlock(MAX_TRANSACTIONS_PER_BLOCK_EXCEEDED_AT_HEIGHT + 1)
	if max != MAX_TRANSACTIONS_PER_BLOCK {
		t.Fatalf("Expected %d at height %d, found",
			MAX_TRANSACTIONS_PER_BLOCK, MAX_TRANSACTIONS_PER_BLOCK_EXCEEDED_AT_HEIGHT+1)
	}
	max = computeMaxTransactionsPerBlock(MAX_TRANSACTIONS_PER_BLOCK_EXCEEDED_AT_HEIGHT - 1)
	if max >= MAX_TRANSACTIONS_PER_BLOCK {
		t.Fatalf("Expected less than max at height %d, found %d",
			MAX_TRANSACTIONS_PER_BLOCK_EXCEEDED_AT_HEIGHT-1, max)
	}
}
