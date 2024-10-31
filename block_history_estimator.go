package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gagliardetto/solana-go/rpc"

	"github.com/smartcontractkit/chainlink-common/pkg/utils/mathutil"
)

const MAX_BLOCK_HISTORY_DEPTH = 20

type EstimationMethod int

const (
	LatestBlock EstimationMethod = iota
	MultipleBlocks
)

func (em EstimationMethod) String() string {
	switch em {
	case LatestBlock:
		return "LatestBlock"
	case MultipleBlocks:
		return "MultipleBlocks"
	default:
		return "UnknownMethod"
	}
}

type BlockHistoryEstimator struct {
	client *rpc.Client
	cfg    Config
	lgr    *log.Logger

	price            uint64
	lock             sync.RWMutex
	stop             chan struct{}
	wg               sync.WaitGroup
	estimationMethod EstimationMethod
	logCounts        LogCounts // Struct to track log counts
}

type LogCounts struct {
	errGetBlockCount   int
	errParseBlockCount int
	errMedianCount     int
}

func NewBlockHistoryEstimator(client *rpc.Client, cfg Config, lgr *log.Logger, method EstimationMethod) *BlockHistoryEstimator {
	return &BlockHistoryEstimator{
		client:           client,
		cfg:              cfg,
		lgr:              lgr,
		price:            cfg.ComputeUnitPriceDefault,
		stop:             make(chan struct{}),
		estimationMethod: method,
		logCounts:        LogCounts{}, // Initialize log counts
	}
}

func (bhe *BlockHistoryEstimator) Start() {
	bhe.wg.Add(1)
	go bhe.run()
	bhe.lgr.Printf("[%s Estimator] Started", bhe.estimationMethod.String())
}

func (bhe *BlockHistoryEstimator) run() {
	defer bhe.wg.Done()
	ticker := time.NewTicker(bhe.cfg.BlockHistoryPollPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-bhe.stop:
			return
		case <-ticker.C:
			if err := bhe.calculatePrice(); err != nil {
				//bhe.lgr.Printf("[%s Estimator] Failed to fetch price: %v", bhe.estimationMethod.String(), err)
			}
		}
	}
}

func (bhe *BlockHistoryEstimator) Close() {
	close(bhe.stop)
	bhe.wg.Wait()
	bhe.lgr.Printf("[%s Estimator] Stopped", bhe.estimationMethod.String())

	// Log the counts of errors encountered during block processing
	fmt.Printf("\n================== Error Analysis ==================\n")
	fmt.Printf("[%s Estimator] Failed to get block: %d times\n", bhe.estimationMethod.String(), bhe.logCounts.errGetBlockCount)
	fmt.Printf("[%s Estimator] Failed to parse block: %d times\n", bhe.estimationMethod.String(), bhe.logCounts.errParseBlockCount)
	fmt.Printf("[%s Estimator] Failed to calculate median: %d times\n", bhe.estimationMethod.String(), bhe.logCounts.errMedianCount)
	fmt.Printf("====================================================\n")
	fmt.Println()
}

func (bhe *BlockHistoryEstimator) BaseComputeUnitPrice() (price uint64, upperBound, lowerBound bool) {
	bhe.lock.RLock()
	defer bhe.lock.RUnlock()
	price = bhe.price
	if price < bhe.cfg.ComputeUnitPriceMin {
		bhe.lgr.Printf("[%s Estimator] BaseComputeUnitPrice: %d is below minimum, using minimum: %d", bhe.estimationMethod.String(), price, bhe.cfg.ComputeUnitPriceMin)
		return bhe.cfg.ComputeUnitPriceMin, false, true
	}
	if price > bhe.cfg.ComputeUnitPriceMax {
		bhe.lgr.Printf("[%s Estimator] BaseComputeUnitPrice: %d is above maximum, using maximum: %d", bhe.estimationMethod.String(), price, bhe.cfg.ComputeUnitPriceMax)
		return bhe.cfg.ComputeUnitPriceMax, true, false
	}
	return price, false, false
}

func (bhe *BlockHistoryEstimator) calculatePrice() error {
	switch bhe.estimationMethod {
	case LatestBlock:
		return bhe.calculatePriceFromLatestBlock()
	case MultipleBlocks:
		return bhe.calculatePriceFromMultipleBlocks(context.Background(), 15)
	default:
		return fmt.Errorf("unknown estimation method")
	}
}

func (bhe *BlockHistoryEstimator) calculatePriceFromLatestBlock() error {
	// Fetch the latest block
	block, err := GetLatestBlock(bhe.client, bhe.cfg.Commitment)
	if err != nil {
		return fmt.Errorf("[%s Estimator] Failed to get latest block: %w", bhe.estimationMethod.String(), err)
	}

	// Parse the block to extract compute unit prices
	feeData, err := ParseBlock(block)
	if err != nil {
		return fmt.Errorf("[%s Estimator] Failed to parse block: %w", bhe.estimationMethod.String(), err)
	}

	// Calculate median price
	medianPrice, err := mathutil.Median(feeData.Prices...)
	if err != nil {
		return fmt.Errorf("[%s Estimator] Failed to calculate median price: %w", bhe.estimationMethod.String(), err)
	}

	// Update the price
	bhe.lock.Lock()
	bhe.price = uint64(medianPrice)
	bhe.lock.Unlock()
	bhe.lgr.Printf(
		"[%s Estimator] Updated computeUnitPriceMedian=%d blockhash=%s slot=%d count=%d",
		bhe.estimationMethod.String(), medianPrice, block.Blockhash, block.ParentSlot+1, len(feeData.Prices),
	)

	return nil
}

func (bhe *BlockHistoryEstimator) calculatePriceFromMultipleBlocks(ctx context.Context, desiredBlockCount uint64) error {
	// Fetch the latest slot
	currentSlot, err := bhe.client.GetSlot(ctx, bhe.cfg.Commitment)
	if err != nil {
		return fmt.Errorf("[%s Estimator] Failed to get current slot: %w", bhe.estimationMethod.String(), err)
	}

	// Determine the starting slot for fetching blocks
	if currentSlot < desiredBlockCount {
		return fmt.Errorf("current slot is less than desired block count")
	}
	startSlot := currentSlot - desiredBlockCount + 1

	// Fetch the last confirmed block slots
	confirmedSlots, err := GetBlocksWithLimit(bhe.client, startSlot, uint64(desiredBlockCount), bhe.cfg.Commitment)
	if err != nil {
		return fmt.Errorf("[%s Estimator] Failed to get blocks with limit: %w", bhe.estimationMethod.String(), err)
	}

	// limit concurrency (avoid hitting rate limits)
	semaphore := make(chan struct{}, 10)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allPrices := make([]ComputeUnitPrice, 0, desiredBlockCount)

	// Iterate over the confirmed slots in reverse order to fetch most recent blocks first
	// Iterate until we run out of slots
	for i := len(*confirmedSlots) - 1; i >= 0; i-- {
		slot := (*confirmedSlots)[i]

		wg.Add(1)
		go func(s uint64) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Fetch the block details
			block, errGetBlock := GetBlock(bhe.client, slot, bhe.cfg.Commitment)
			if errGetBlock != nil || block == nil {
				// Failed to get block at slot || no block found at slot: skip.
				mu.Lock()
				bhe.logCounts.errGetBlockCount++ // Increment the failure counter
				mu.Unlock()
				bhe.lgr.Printf("[%s Estimator] Failed to get block at slot %d: %v", bhe.estimationMethod.String(), slot, errGetBlock)
				return
			}

			// Parse the block to extract compute unit prices
			feeData, errParseBlock := ParseBlock(block)
			if errParseBlock != nil {
				// Failed to parse block at slot: skip.
				mu.Lock()
				bhe.logCounts.errParseBlockCount++ // Increment the failure counter
				mu.Unlock()
				bhe.lgr.Printf("[%s Estimator] Failed to parse block at slot %d: %v", bhe.estimationMethod.String(), slot, errParseBlock)
				return
			}

			if len(feeData.Prices) == 0 {
				// No relevant transactions for compute unit price found in this block
				return
			}

			// Calculate the median compute unit price for the block
			//blockMedian, errMedian := mathutil.Avg(feeData.Prices...)
			blockMedian, errMedian := mathutil.Median(feeData.Prices...)
			if errMedian != nil {
				// Log the block, slot and feeData that had an error
				bhe.lgr.Printf("[%s Estimator] Failed to calculate median for block %v at slot %d: %v", bhe.estimationMethod.String(), block, slot, errMedian)
				//Failed to calculate median for slot: skip.
				mu.Lock()
				bhe.logCounts.errMedianCount++ // Increment the failure counter
				mu.Unlock()
				bhe.lgr.Printf("[%s Estimator] Failed to calculate median for block at slot %d: %v", bhe.estimationMethod.String(), slot, errMedian)
				return
			}

			// Append the median compute unit price if we haven't reached our desiredBlockCount
			mu.Lock()
			defer mu.Unlock()
			if uint64(len(allPrices)) < desiredBlockCount {
				allPrices = append(allPrices, blockMedian)
			}
		}(slot)
	}

	wg.Wait()

	if len(allPrices) == 0 {
		return fmt.Errorf("no compute unit prices collected")
	}

	// Calculate the median of all collected compute unit prices
	medianPrice, err := mathutil.Median(allPrices...)
	if err != nil {
		return fmt.Errorf("failed to calculate median price: %w", err)
	}

	// Update the current price to the median of the last desiredBlockCount
	bhe.lock.Lock()
	bhe.price = uint64(medianPrice)
	bhe.lock.Unlock()

	bhe.lgr.Printf(
		"[%s Estimator] Updated computeUnitPriceMedian=%d latestSlot=%d count=%d allPrices=%v",
		bhe.estimationMethod.String(), medianPrice, currentSlot, len(allPrices), allPrices,
	)

	return nil
}
