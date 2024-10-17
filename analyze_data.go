package main

import (
	"fmt"
	"log"
	"time"

	"github.com/gagliardetto/solana-go"
)

type TransactionData struct {
	EstimatorType      string
	Signature          solana.Signature
	Timestamp          time.Time
	ComputeUnitPrice   uint64
	BlockMedianPrice   uint64
	InclusionStatus    bool
	SentTimestamp      time.Time
	InclusionTimestamp time.Time
	Duration           time.Duration
	Error              error
	BoundedUp          bool
	BoundedDown        bool
}

func analyzeData(latestBlockData []*TransactionData, multipleBlocksData []*TransactionData, lgr *log.Logger) {
	if len(latestBlockData) != len(multipleBlocksData) {
		lgr.Println("Mismatch in the number of transactions between the two estimators.")
		return
	}

	totalTransactions := len(latestBlockData)
	fmt.Println("====================== Analysis Results ======================")
	fmt.Printf("Total Transactions: %d\n", totalTransactions)

	// Initialize metrics for both estimators
	metrics := map[string]struct {
		includedCount     int
		totalDuration     time.Duration
		overpaidCount     int
		underpaidCount    int
		boundedUpCount    int
		boundedDownCount  int
		totalComputePrice uint64
	}{
		"LatestBlock":    {},
		"MultipleBlocks": {},
	}

	// Analyze each transaction pair
	for i := 0; i < totalTransactions; i++ {
		for _, data := range []*TransactionData{latestBlockData[i], multipleBlocksData[i]} {
			m := metrics[data.EstimatorType]
			if data.InclusionStatus {
				m.includedCount++
				m.totalDuration += data.Duration
				m.totalComputePrice += data.ComputeUnitPrice

				if data.BoundedUp {
					m.boundedUpCount++
				}
				if data.BoundedDown {
					m.boundedDownCount++
				}
				metrics[data.EstimatorType] = m
			}
		}
	}

	// Calculate and display metrics for each estimator
	for _, estimator := range []string{"LatestBlock", "MultipleBlocks"} {
		m := metrics[estimator]
		successRate := float64(m.includedCount) / float64(totalTransactions) * 100
		avgDuration := m.totalDuration / time.Duration(totalTransactions)
		avgComputePrice := m.totalComputePrice / uint64(totalTransactions)
		//boundedUpRate := float64(m.boundedUpCount) / float64(totalTransactions) * 100
		//boundedDownRate := float64(m.boundedDownCount) / float64(totalTransactions) * 100
		//totalBoundedCount := m.boundedUpCount + m.boundedDownCount
		//totalBoundedRate := boundedUpRate + boundedDownRate

		fmt.Println()
		fmt.Printf("\n====================== %s Estimator ======================\n", estimator)
		fmt.Printf("Success Rate: %.2f%% (%d/%d)\n", successRate, m.includedCount, totalTransactions)
		fmt.Printf("Average Inclusion Time: %v\n", avgDuration)
		fmt.Printf("Average Compute Unit Price: %d\n", avgComputePrice)
		fmt.Printf("Total Compute Unit Price Used: %d\n", m.totalComputePrice)
		//fmt.Printf("Transactions Bounded By Config: %d (%.2f%%)\n", totalBoundedCount, totalBoundedRate)
		//fmt.Printf("  - Bounded Up (Adjusted Down to Max): %d (%.2f%%)\n", m.boundedUpCount, boundedUpRate)
		//fmt.Printf("  - Bounded Down (Adjusted Up to Min): %d (%.2f%%)\n", m.boundedDownCount, boundedDownRate)
	}

	// Comparison between estimators
	fmt.Println()
	fmt.Println("====================== Comparison ======================")
	lbMetrics := metrics["LatestBlock"]
	mbMetrics := metrics["MultipleBlocks"]

	// Success Rate Comparison
	lbSuccessRate := float64(lbMetrics.includedCount) / float64(totalTransactions) * 100
	mbSuccessRate := float64(mbMetrics.includedCount) / float64(totalTransactions) * 100

	if lbMetrics.includedCount > mbMetrics.includedCount {
		fmt.Printf("LatestBlock Estimator has a higher success rate: %.2f%% (%d/%d) compared to MultipleBlocks Estimator: %.2f%% (%d/%d).\n",
			lbSuccessRate, lbMetrics.includedCount, totalTransactions,
			mbSuccessRate, mbMetrics.includedCount, totalTransactions)
	} else if lbMetrics.includedCount < mbMetrics.includedCount {
		fmt.Printf("MultipleBlocks Estimator has a higher success rate: %.2f%% (%d/%d) compared to LatestBlock Estimator: %.2f%% (%d/%d).\n",
			mbSuccessRate, mbMetrics.includedCount, totalTransactions,
			lbSuccessRate, lbMetrics.includedCount, totalTransactions)
	} else {
		fmt.Printf("Both estimators have the same success rate: %.2f%% (%d/%d).\n",
			lbSuccessRate, lbMetrics.includedCount, totalTransactions)
	}

	// Average Inclusion Time Comparison
	lbAvgDuration := lbMetrics.totalDuration / time.Duration(lbMetrics.includedCount)
	mbAvgDuration := mbMetrics.totalDuration / time.Duration(mbMetrics.includedCount)
	if lbAvgDuration < mbAvgDuration {
		fmt.Printf("LatestBlock Estimator has a faster average inclusion time: %v compared to MultipleBlocks Estimator: %v.\n", lbAvgDuration, mbAvgDuration)
	} else if lbAvgDuration > mbAvgDuration {
		fmt.Printf("MultipleBlocks Estimator has a faster average inclusion time: %v compared to LatestBlock Estimator: %v.\n", mbAvgDuration, lbAvgDuration)
	} else {
		fmt.Printf("Both estimators have the same average inclusion time: %v.\n", lbAvgDuration)
	}

	// Total Compute Unit Price Comparison
	if lbMetrics.totalComputePrice < mbMetrics.totalComputePrice {
		fmt.Printf("LatestBlock Estimator used less total compute unit price: %d compared to MultipleBlocks Estimator: %d.\n",
			lbMetrics.totalComputePrice, mbMetrics.totalComputePrice)
	} else if lbMetrics.totalComputePrice > mbMetrics.totalComputePrice {
		fmt.Printf("MultipleBlocks Estimator used less total compute unit price: %d compared to LatestBlock Estimator: %d.\n",
			mbMetrics.totalComputePrice, lbMetrics.totalComputePrice)
	} else {
		fmt.Printf("Both estimators used the same total compute unit price: %d.\n", lbMetrics.totalComputePrice)
	}

	// Bounded by Config Comparison
	/* 	lbTotalBoundedCount := lbMetrics.boundedUpCount + lbMetrics.boundedDownCount
	mbTotalBoundedCount := mbMetrics.boundedUpCount + mbMetrics.boundedDownCount

	if lbTotalBoundedCount < mbTotalBoundedCount {
		fmt.Printf("LatestBlock Estimator exceeded config bounds less frequently: %d times compared to MultipleBlocks Estimator: %d times.\n",
		lbTotalBoundedCount, mbTotalBoundedCount)
		} else if lbTotalBoundedCount > mbTotalBoundedCount {
			fmt.Printf("MultipleBlocks Estimator exceeded config bounds less frequently: %d times compared to LatestBlock Estimator: %d times.\n",
			mbTotalBoundedCount, lbTotalBoundedCount)
			} else {
				fmt.Printf("Both estimators exceeded config bounds equally: %d times.\n", lbTotalBoundedCount)
				} */
	fmt.Println()
	fmt.Println()
}
