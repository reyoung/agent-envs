import datasets
import argparse
import json

def main():
    parser = argparse.ArgumentParser(description="Convert datasets to JSONL format.")
    parser.add_argument("--dataset", type=str, required=True, help="Name of the dataset to convert.")
    parser.add_argument("--split", type=str, help="Dataset split to convert (e.g., train, test). If not specified, all splits are converted.")
    parser.add_argument("--output", required=True, help="Output JSONL file path.")
    args = parser.parse_args()
    with open(args.output, 'w', encoding='utf-8') as out_file:
        ds = datasets.load_dataset(args.dataset, split=args.split)

        if args.split is None:
            for val in ds.values():
                for item in val:
                    json.dump(item, out_file)
                    out_file.write('\n')
            
        else:
            for item in ds:
                json.dump(item, out_file)
                out_file.write('\n')

if __name__ == "__main__":
    main()