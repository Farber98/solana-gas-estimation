import os
import json

directory = './'  
all_blocks = []

# Loop through all files in the directory
for filename in os.listdir(directory):
    if filename.endswith(".json") and filename.startswith("block_"):
        filepath = os.path.join(directory, filename)

        # Open and load each JSON file
        with open(filepath, 'r') as file:
            try:
                data = json.load(file)
                if data:  # Only append if the file is not empty
                    all_blocks.append(data)
            except json.JSONDecodeError:
                print(f"Error reading {filename}, skipping...")

# Save all merged blocks into one JSON file
with open('merged_blocks.json', 'w') as outfile:
    json.dump(all_blocks, outfile, indent=4)

print(f"All blocks merged into merged_blocks.json")
