package main

import (
	"math"
	"time"

	"github.com/gagliardetto/solana-go/rpc"
)

type Config struct {
	ComputeUnitPriceDefault uint64
	ComputeUnitPriceMin     uint64
	ComputeUnitPriceMax     uint64
	BlockHistoryPollPeriod  time.Duration
	Commitment              rpc.CommitmentType
}

func getConfig() Config {
	return Config{
		ComputeUnitPriceDefault: 0,
		ComputeUnitPriceMin:     0,
		ComputeUnitPriceMax:     math.MaxUint64,
		BlockHistoryPollPeriod:  5 * time.Second,
		Commitment:              rpc.CommitmentConfirmed,
	}
}
