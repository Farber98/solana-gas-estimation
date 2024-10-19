# Solana Transaction Analyzer

This project analyzes Solana blockchain transactions using different estimation methods. It includes scripts to download and merge block data, and Go code to build, send, and analyze transactions through different gas estimation methods.

## Helper Scripts

### Download Solana Blocks
Fetches and saves recent blocks from the Solana blockchain.

### Merge Block Data
Merges individual block JSON files into a single file.

## Analyzer
### Main Functions
- main.go: Entry point. Initializes estimators and sends transactions.
- analyze_data.go: Analyzes transaction data.
- block_history_estimator.go: Estimates compute unit prices using different methods.

### Key Functions
- buildTransaction: Constructs and signs a transaction.
- sendTransaction: Builds, sends, and tracks a transaction.
- analyzeData: Analyzes transaction data for different estimators.