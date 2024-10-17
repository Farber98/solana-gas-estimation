package main

import (
	"context"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"github.com/gagliardetto/solana-go/rpc"
)

type BlockData struct {
	Fees   []uint64           // total fee
	Prices []ComputeUnitPrice // price per unit
}

func ParseBlock(res *rpc.GetBlockResult) (out BlockData, err error) {
	if res == nil {
		return out, fmt.Errorf("GetBlockResult was nil")
	}

	for _, tx := range res.Transactions {
		if tx.Meta == nil {
			continue
		}
		baseTx, getTxErr := tx.GetTransaction()
		if getTxErr != nil {
			// exit on GetTransaction error
			// if this occurs, solana-go was unable to parse a transaction
			// further investigation is required to determine if there is incompatibility
			return out, fmt.Errorf("failed to GetTransaction (blockhash: %s): %w", res.Blockhash, err)
		}
		if baseTx == nil {
			continue
		}

		// filter out consensus vote transactions
		// consensus messages are included as txs within blocks
		if len(baseTx.Message.Instructions) == 1 &&
			baseTx.Message.AccountKeys[baseTx.Message.Instructions[0].ProgramIDIndex] == solana.VoteProgramID {
			continue
		}

		var price ComputeUnitPrice // default 0
		for _, instruction := range baseTx.Message.Instructions {
			// find instructions for compute budget program
			if baseTx.Message.AccountKeys[instruction.ProgramIDIndex] == ComputeBudgetProgram {
				parsed, parseErr := ParseComputeUnitPrice(instruction.Data)
				// if compute unit price found, break instruction loop
				// only one compute unit price tx is allowed
				// err returned if not SetComputeUnitPrice instruction
				if parseErr == nil {
					price = parsed
					break
				}
			}
		}
		out.Prices = append(out.Prices, price)
		out.Fees = append(out.Fees, tx.Meta.Fee)
	}
	return out, nil
}

func GetLatestBlock(client *rpc.Client, commitment rpc.CommitmentType) (*rpc.GetBlockResult, error) {
	// Get the latest confirmed slot
	ctx := context.Background()
	slot, err := client.GetSlot(ctx, commitment)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest slot: %w", err)
	}

	// Fetch the block for that slot
	version := uint64(0) // Pull all transaction versions
	block, err := client.GetBlockWithOpts(ctx, slot, &rpc.GetBlockOpts{
		Commitment:                     commitment,
		MaxSupportedTransactionVersion: &version,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}

	return block, nil
}

func GetBlock(client *rpc.Client, slot uint64, commitment rpc.CommitmentType) (*rpc.GetBlockResult, error) {
	ctx := context.Background()

	// Set block fetch options
	version := uint64(0) // Pull all transaction versions
	// Fetch the block at the specified slot
	block, err := client.GetBlockWithOpts(ctx, slot, &rpc.GetBlockOpts{
		Commitment:                     commitment,
		MaxSupportedTransactionVersion: &version, // Fetch all transaction versions
	})

	if err != nil {
		return nil, fmt.Errorf("failed to get block at slot %d: %w", slot, err)
	}

	return block, nil
}

func GetBlocksWithLimit(client *rpc.Client, slot, limit uint64, commitment rpc.CommitmentType) (*rpc.BlocksResult, error) {
	ctx := context.Background()

	// Fetch the block at the specified slot
	blocks, err := client.GetBlocksWithLimit(ctx, slot, limit, commitment)

	if err != nil {
		return nil, fmt.Errorf("failed to get blocks from slot %d to limit %d: %w", slot, limit, err)
	}

	return blocks, nil
}
