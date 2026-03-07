import os
import argparse
from huggingface_hub import snapshot_download
from modelscope.hub.snapshot_download import snapshot_download as ms_snapshot_download

MODELS = {
    "qwen": "Qwen/Qwen2.5-0.5B-Instruct",
    "llama": "meta-llama/Llama-3.2-1B-Instruct",
}

MS_MODELS = {
    "qwen": "qwen/Qwen2.5-0.5B-Instruct",
}

def download_huggingface(model_id, local_dir):
    print(f"Downloading {model_id} from HuggingFace to {local_dir}...")
    snapshot_download(repo_id=model_id, local_dir=local_dir, ignore_patterns=["*.safetensors", "*.msgpack", "*.h5"])
    print("Done!")

def download_modelscope(model_id, local_dir):
    print(f"Downloading {model_id} from ModelScope to {local_dir}...")
    ms_snapshot_download(model_id=model_id, local_dir=local_dir, ignore_file_pattern=["*.safetensors", "*.msgpack", "*.h5"])
    print("Done!")

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Download local models for testing Forge")
    parser.add_argument("--model", type=str, choices=list(MODELS.keys()), default="qwen", help="Model to download")
    parser.add_argument("--source", type=str, choices=["hf", "ms"], default="hf", help="Source to download from (hf=HuggingFace, ms=ModelScope)")
    parser.add_argument("--dir", type=str, default="./models", help="Directory to save models")

    args = parser.parse_args()

    model_name = args.model
    local_dir = os.path.join(args.dir, model_name)
    os.makedirs(local_dir, exist_ok=True)

    if args.source == "hf":
        if model_name in MODELS:
            download_huggingface(MODELS[model_name], local_dir)
        else:
            print(f"Model {model_name} not available on HuggingFace")
    elif args.source == "ms":
        if model_name in MS_MODELS:
            download_modelscope(MS_MODELS[model_name], local_dir)
        else:
            print(f"Model {model_name} not available on ModelScope")
