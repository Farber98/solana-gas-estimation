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
}

func NewBlockHistoryEstimator(client *rpc.Client, cfg Config, lgr *log.Logger, method EstimationMethod) *BlockHistoryEstimator {
	return &BlockHistoryEstimator{
		client:           client,
		cfg:              cfg,
		lgr:              lgr,
		price:            cfg.ComputeUnitPriceDefault,
		stop:             make(chan struct{}),
		estimationMethod: method,
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
		return bhe.calculatePriceFromMultipleBlocks()
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

func (bhe *BlockHistoryEstimator) calculatePriceFromMultipleBlocks() error {
	ctx := context.Background()

	// Fetch the latest slot
	currentSlot, err := bhe.client.GetSlot(ctx, bhe.cfg.Commitment)
	if err != nil {
		return fmt.Errorf("[%s Estimator] Failed to get current slot: %w", bhe.estimationMethod.String(), err)
	}

	desiredBlockCount := 15
	var allPrices []ComputeUnitPrice
	var wg sync.WaitGroup
	var mu sync.Mutex

	// Determine the starting slot for fetching blocks
	startSlot := currentSlot - uint64(desiredBlockCount) + 1

	// Fetch the last confirmed block slots using getBlocksWithLimit
	confirmedSlots, err := GetBlocksWithLimit(bhe.client, startSlot, uint64(desiredBlockCount), bhe.cfg.Commitment)
	if err != nil {
		return fmt.Errorf("[%s Estimator] Failed to get blocks with limit: %w", bhe.estimationMethod.String(), err)
	}

	// Implement a semaphore to limit concurrency (e.g., 10 concurrent goroutines)
	semaphore := make(chan struct{}, 10)

	for i := len(*confirmedSlots) - 1; i >= 0 && len(allPrices) < desiredBlockCount; i-- {
		slot := (*confirmedSlots)[i]

		wg.Add(1)
		go func(s uint64) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Fetch the block details
			block, err := GetBlock(bhe.client, s, bhe.cfg.Commitment)
			if err != nil {
				bhe.lgr.Printf("[%s Estimator] Failed to get block at slot %d: %v", bhe.estimationMethod.String(), s, err)
				return
			}

			if block == nil {
				bhe.lgr.Printf("[%s Estimator] No block found at slot %d, skipping.", bhe.estimationMethod.String(), s)
				return
			}

			// Parse the block to extract compute unit prices
			feeData, err := ParseBlock(block)
			if err != nil {
				bhe.lgr.Printf("[%s Estimator] Failed to parse block at slot %d: %v", bhe.estimationMethod.String(), s, err)
				return
			}

			// Calculate the median compute unit price for the block
			blockMedian, err := mathutil.Median(feeData.Prices...)
			if err != nil {
				//bhe.lgr.Printf("[%s Estimator] Failed to calculate median for slot %d: %v", bhe.estimationMethod.String(), s, err)
				return
			}

			// Append the median compute unit price
			mu.Lock()
			if len(allPrices) < desiredBlockCount {
				allPrices = append(allPrices, blockMedian)
				//bhe.lgr.Printf("[%s Estimator] Collected compute unit price %d from slot %d", bhe.estimationMethod.String(), blockMedian, s)
			}
			mu.Unlock()
		}(slot)
	}

	wg.Wait()

	// Calculate the median of all collected compute unit prices
	medianPrice, err := mathutil.Median(allPrices...)
	if err != nil {
		return fmt.Errorf("[%s Estimator] Failed to calculate median price: %w", bhe.estimationMethod.String(), err)
	}

	// Update the current price to the median of the last 10 blocks
	bhe.lock.Lock()
	bhe.price = uint64(medianPrice)
	bhe.lock.Unlock()

	bhe.lgr.Printf(
		"[%s Estimator] Updated computeUnitPriceMedian=%d latestSlot=%d count=%d allPrices=%v",
		bhe.estimationMethod.String(), medianPrice, currentSlot, len(allPrices), allPrices,
	)

	return nil
}
