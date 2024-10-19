import requests
import json

RPC_URL = "https://api.devnet.solana.com" 

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

def save_block_as_json(slot, block_data):
    filename = f"block_{slot}.json"
    with open(filename, "w") as json_file:
        json.dump(block_data, json_file, indent=4)  
    print(f"Block {slot} saved as {filename}")

# Main process to fetch recent blocks
def download_recent_blocks(number_of_blocks=5):
    latest_slot = get_latest_slot()

    if latest_slot is None:
        print("Error: Unable to fetch the latest slot")
        return

    # Fetch and save the latest blocks
    for slot in range(latest_slot - number_of_blocks + 1, latest_slot + 1):
        block_data = fetch_block(slot)
        if "result" in block_data:
            save_block_as_json(slot, block_data["result"])
        else:
            print(f"Error fetching block {slot}: {block_data}")

# Fetch the latest n blocks 
download_recent_blocks(5)
