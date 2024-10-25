import requests
import json

RPC_URL = "https://api.devnet.solana.com"

# Function to get the latest slot number
def get_latest_slot():
    headers = {"Content-Type": "application/json"}
    data = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "getSlot",  # Get the most recent slot
        "params": []
    }
    response = requests.post(RPC_URL, headers=headers, json=data)
    result = response.json()
    return result.get("result", None)

# Function to fetch block data by slot number using getBlock with base64-encoded transactions
def fetch_block(slot):
    headers = {"Content-Type": "application/json"}
    data = {
        "jsonrpc": "2.0",
        "id": 1,
        "method": "getBlock",
        "params": [
            slot,
            {
                "encoding": "base64",  
                "transactionDetails": "full",
                "rewards": False,  
                "maxSupportedTransactionVersion": 0 
            }
        ]
    }
    response = requests.post(RPC_URL, headers=headers, json=data)
    return response.json()

# Function to filter transactions that reference the ComputeBudget program
def filter_transactions(transactions):
    filtered_transactions = []
    for transaction in transactions:
        log_messages = transaction.get('meta', {}).get('logMessages', [])
        if any("Program ComputeBudget111111111111111111111111111111" in log for log in log_messages):
            filtered_transactions.append(transaction)
    return filtered_transactions

# Function to save block data after filtering transactions
def save_filtered_block_as_json(slot, block_data):
    if block_data and "transactions" in block_data:
        # Filter the transactions
        filtered_transactions = filter_transactions(block_data["transactions"])
        
        # Only save the block if it contains relevant transactions
        if filtered_transactions:
            block_data["transactions"] = filtered_transactions
            filename = f"block_{slot}_filtered.json"
            with open(filename, "w") as json_file:
                json.dump(block_data, json_file, indent=4)
            print(f"Block {slot} saved as {filename} with {len(filtered_transactions)} relevant transactions")
        else:
            print(f"Block {slot} had no relevant transactions and was skipped.")
    else:
        print(f"Block {slot} data was incomplete or invalid, skipping.")

# Main process to fetch recent blocks and filter transactions
def download_recent_blocks(number_of_blocks=5):
    latest_slot = get_latest_slot()

    if latest_slot is None:
        print("Error: Unable to fetch the latest slot")
        return

    # Fetch and save the latest blocks
    for slot in range(latest_slot - number_of_blocks + 1, latest_slot + 1):
        block_data = fetch_block(slot)
        if "result" in block_data:
            save_filtered_block_as_json(slot, block_data["result"])
        else:
            print(f"Error fetching block {slot}: {block_data}")

# Fetch the latest n blocks
download_recent_blocks(5)
