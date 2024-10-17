package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	compute_budget "github.com/gagliardetto/solana-go/programs/compute-budget"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/programs/system"
	"github.com/gagliardetto/solana-go/rpc"
)

var endpoint = "https://api.devnet.solana.com"

func getKeys() (solana.PrivateKey, solana.PrivateKey) {
	// Create a new RPC client
	senderKeyPair := "sender.json"
	recipientKeyPair := "recipient.json"

	// Load the sender's private key
	senderPrivateKey, err := solana.PrivateKeyFromSolanaKeygenFile(senderKeyPair)
	if err != nil {
		log.Fatalf("Failed to read sender keypair: %v", err)
	}

	// Load the recipient private key
	recipientPrivateKey, err := solana.PrivateKeyFromSolanaKeygenFile(recipientKeyPair)
	if err != nil {
		log.Fatalf("Failed to read recipient keypair: %v", err)

	}

	// Get public keys
	return senderPrivateKey, recipientPrivateKey
}

func main() {
	// Load the keys
	senderPrivateKey, recipientPrivateKey := getKeys()

	//fmt.Printf("Recipient Public Key: %s\n", recipientPrivateKey.PublicKey().String())
	//fmt.Printf("Sender Public Key: %s\n", senderPrivateKey.PublicKey().String())

	// Set up the context
	ctx := context.Background()
	client := rpc.New(endpoint)

	// Initialize logger
	lgr := log.Default()

	// Initialize the estimator
	cfg := getConfig()

	// Initialize estimators
	// Start estimators
	estimatorLatestBlock := NewBlockHistoryEstimator(client, cfg, lgr, LatestBlock)
	estimatorMultipleBlocks := NewBlockHistoryEstimator(client, cfg, lgr, MultipleBlocks)
	estimatorLatestBlock.Start()
	estimatorMultipleBlocks.Start()
	defer estimatorLatestBlock.Close()
	defer estimatorMultipleBlocks.Close()

	// Variables to collect data
	var (
		transactionDataListLatestBlock    []*TransactionData
		transactionDataListMultipleBlocks []*TransactionData
	)

	time.Sleep(20 * time.Second) // Wait for estimators to warm up

	// Number of transactions to send
	numTransactions := 100
	for i := 0; i < numTransactions; i++ {
		// Get compute unit prices from both estimators (may be bounded)
		computeUnitPriceLatest, boundedUpLatest, boundedDownLatest := estimatorLatestBlock.BaseComputeUnitPrice()
		computeUnitPriceMultiple, boundedUpMultiple, boundedDownMultiple := estimatorMultipleBlocks.BaseComputeUnitPrice()

		fmt.Println("=====================================================")
		fmt.Printf("Transaction %d:\n", i+1)
		fmt.Printf("  LatestBlock Estimator - Compute Unit Price: %d\n", computeUnitPriceLatest)
		fmt.Printf("  MultipleBlocks Estimator - Compute Unit Price: %d\n", computeUnitPriceMultiple)
		fmt.Println("=====================================================")

		// Specify the amount to transfer (in lamports)
		amount := uint64(1000000) // 0.001 SOL

		// Use WaitGroup to wait for both transactions to complete
		var wg sync.WaitGroup
		wg.Add(2) // Two transactions

		// Channels to collect transaction data
		latestBlockChan := make(chan *TransactionData, 1)
		multipleBlocksChan := make(chan *TransactionData, 1)

		// Get the latest blockhash 1
		latestBlockhashResult1, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
		if err != nil {
			lgr.Printf("Transaction %d: Failed to get latest blockhash: %v", i+1, err)
			return
		}
		latestBlockhash1 := latestBlockhashResult1.Value.Blockhash

		// minimum difference so we get a different blockhash.
		time.Sleep(500 * time.Millisecond)

		// Get the latest blockhash 2
		latestBlockhashResult2, err := client.GetLatestBlockhash(ctx, rpc.CommitmentFinalized)
		if err != nil {
			lgr.Printf("Transaction %d: Failed to get latest blockhash: %v", i+1, err)
			return
		}
		latestBlockhash2 := latestBlockhashResult2.Value.Blockhash

		// Send LatestBlock transaction concurrently
		go func() {
			defer wg.Done()
			txData := sendTransaction(
				ctx,
				client,
				senderPrivateKey,
				recipientPrivateKey.PublicKey(),
				amount,
				latestBlockhash1,
				computeUnitPriceLatest,
				"LatestBlock",
				boundedUpLatest,
				boundedDownLatest,
				lgr,
				i,
			)
			latestBlockChan <- txData
		}()

		// Send MultipleBlocks transaction concurrently
		go func() {
			defer wg.Done()
			txData := sendTransaction(
				ctx,
				client,
				senderPrivateKey,
				recipientPrivateKey.PublicKey(),
				amount,
				latestBlockhash2,
				computeUnitPriceMultiple,
				"MultipleBlocks",
				boundedUpMultiple,
				boundedDownMultiple,
				lgr,
				i,
			)
			multipleBlocksChan <- txData
		}()

		// Wait for both transactions to complete
		wg.Wait()
		close(latestBlockChan)
		close(multipleBlocksChan)
		// Collect transaction data
		transactionDataListLatestBlock = append(transactionDataListLatestBlock, <-latestBlockChan)
		transactionDataListMultipleBlocks = append(transactionDataListMultipleBlocks, <-multipleBlocksChan)
	}

	// Analyze the collected data
	fmt.Println()
	analyzeData(transactionDataListLatestBlock, transactionDataListMultipleBlocks, lgr)
}

// sendTransaction builds, sends, and tracks a transaction.
func sendTransaction(
	ctx context.Context,
	client *rpc.Client,
	senderPrivateKey solana.PrivateKey,
	recipientPublicKey solana.PublicKey,
	amount uint64,
	recentBlockhash solana.Hash,
	computeUnitPrice uint64,
	estimatorType string,
	boundedUp bool,
	boundedDown bool,
	lgr *log.Logger,
	idx int,
) *TransactionData {
	// Build the transaction
	tx, err := buildTransaction(senderPrivateKey, recipientPublicKey, amount, recentBlockhash, computeUnitPrice)
	if err != nil {
		lgr.Printf("%s Estimator: Failed to build transaction: %v", estimatorType, err)
		return &TransactionData{
			EstimatorType:    estimatorType,
			Error:            err,
			ComputeUnitPrice: computeUnitPrice,
			BoundedUp:        boundedUp,
			BoundedDown:      boundedDown,
		}
	}

	// Send the transaction
	sentTimestamp := time.Now()
	txhash, err := client.SendTransaction(ctx, tx)
	if err != nil {
		lgr.Printf("%s Estimator: Failed to send transaction: %v", estimatorType, err)
		return &TransactionData{
			EstimatorType:    estimatorType,
			Error:            err,
			ComputeUnitPrice: computeUnitPrice,
		}
	}

	lgr.Printf("%s Estimator: Sent transaction %d!", estimatorType, idx+1)

	// Check for transaction confirmation and record inclusion timestamp
	inclusionStatus, inclusionTime := checkTransactionInclusion(ctx, client, txhash, lgr)

	// Calculate duration
	var duration time.Duration
	if inclusionStatus {
		duration = inclusionTime.Sub(sentTimestamp)
		lgr.Printf("%s Estimator: Transaction %d finalized! Duration: %v", estimatorType, idx, duration)
	} else {
		lgr.Printf("%s Estimator: Transaction %d inclusion failed or timed out.", estimatorType, idx)
	}

	// Collect transaction data
	return &TransactionData{
		EstimatorType:      estimatorType,
		Signature:          txhash,
		SentTimestamp:      sentTimestamp,
		InclusionTimestamp: inclusionTime,
		Duration:           duration,
		ComputeUnitPrice:   computeUnitPrice,
		InclusionStatus:    inclusionStatus,
		Error:              err,
		BoundedUp:          boundedUp,
		BoundedDown:        boundedDown,
	}
}

// buildTransaction constructs and signs a transaction with the given parameters.
func buildTransaction(senderPrivateKey solana.PrivateKey, recipientPublicKey solana.PublicKey, amount uint64, recentBlockhash solana.Hash, computeUnitPrice uint64) (*solana.Transaction, error) {
	// Create the transfer instruction
	transferInstruction := system.NewTransferInstruction(
		amount,
		senderPrivateKey.PublicKey(),
		recipientPublicKey,
	).Build()

	// Create the compute budget instruction
	computeBudgetInstruction := compute_budget.NewSetComputeUnitPriceInstruction(computeUnitPrice).Build()

	// Build the transaction using TransactionBuilder
	txBuilder := solana.NewTransactionBuilder()
	txBuilder.SetRecentBlockHash(recentBlockhash)
	txBuilder.SetFeePayer(senderPrivateKey.PublicKey())
	txBuilder.AddInstruction(computeBudgetInstruction)
	txBuilder.AddInstruction(transferInstruction)

	// Build the transaction
	tx, err := txBuilder.Build()
	if err != nil {
		return nil, fmt.Errorf("failed to build transaction: %w", err)
	}

	// Sign the transaction
	_, err = tx.Sign(
		func(key solana.PublicKey) *solana.PrivateKey {
			if senderPrivateKey.PublicKey().Equals(key) {
				return &senderPrivateKey
			}
			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	return tx, nil
}

func checkTransactionInclusion(ctx context.Context, client *rpc.Client, signature solana.Signature, lgr *log.Logger) (bool, time.Time) {
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			lgr.Printf("Transaction %s: Timeout waiting for confirmation.", signature)
			return false, time.Time{}
		case <-ticker.C:
			// Check transaction status
			resp, err := client.GetSignatureStatuses(ctx, true, signature)
			if err != nil {
				lgr.Printf("Error checking transaction status: %v", err)
				continue
			}
			if resp == nil || len(resp.Value) == 0 || resp.Value[0] == nil {
				continue
			}
			status := resp.Value[0]
			if status.ConfirmationStatus == rpc.ConfirmationStatusFinalized {
				return true, time.Now()
			}
		}
	}
}
