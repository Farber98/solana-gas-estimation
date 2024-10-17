package main

import (
	"encoding/binary"
	"fmt"

	"github.com/gagliardetto/solana-go"
	"golang.org/x/exp/constraints"
)

type ComputeUnitPrice uint64
type computeBudgetInstruction uint8

var (
	ComputeBudgetProgram = solana.MustPublicKeyFromBase58("ComputeBudget111111111111111111111111111111")
)

// Set a compute unit price in "micro-lamports" to pay a higher transaction
// fee for higher transaction prioritization.
// note: uses ag_binary.Uint64
const InstructionSetComputeUnitPrice = 3

func ParseComputeUnitPrice(data []byte) (ComputeUnitPrice, error) {
	v, err := parse(InstructionSetComputeUnitPrice, data, binary.LittleEndian.Uint64)
	return ComputeUnitPrice(v), err
}

// parse implements tx data parsing for the provided instruction type and specified decoder
func parse[V constraints.Unsigned](ins computeBudgetInstruction, data []byte, decoder func([]byte) V) (V, error) {
	if len(data) != (1 + binary.Size(V(0))) { // instruction byte + uintXXX length
		return 0, fmt.Errorf("invalid length: %d", len(data))
	}

	// validate instruction identifier
	if data[0] != uint8(ins) {
		return 0, fmt.Errorf("not %s identifier: %d", ins, data[0])
	}

	// guarantees length to fit the binary decoder
	return decoder(data[1:]), nil
}
