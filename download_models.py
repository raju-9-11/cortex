import os
import argparse
from huggingface_hub import snapshot_download
from modelscope.hub.snapshot_download import snapshot_download as ms_snapshot_download

MODELS = {
    "qwen": "bartowski/Qwen2.5-0.5B-Instruct-GGUF",
    "llama": "bartowski/Llama-3.2-1B-Instruct-GGUF",
    "uncensored": "bartowski/mlabonne_gemma-3-12b-it-abliterated-GGUF",
    "multimax": "bartowski/MiniMax-M2.5-GGUF",
}

MS_MODELS = {
    "qwen": "qwen/Qwen2.5-0.5B-Instruct-GGUF",
}

if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Download local models for testing Cortex")
    parser.add_argument("--model", type=str, choices=list(MODELS.keys()), default="qwen", help="Model to download")
    parser.add_argument("--source", type=str, choices=["hf", "ms"], default="hf", help="Source to download from (hf=HuggingFace, ms=ModelScope)")
    parser.add_argument("--dir", type=str, default="./models", help="Directory to save models")
    parser.add_argument("--quant", type=str, default="Q4_K_M", help="Quantization to download (e.g. Q4_K_M)")

    args = parser.parse_args()

    model_name = args.model
    local_dir = os.path.join(args.dir, model_name)
    os.makedirs(local_dir, exist_ok=True)

    # Support split files (e.g. *-00001-of-00009.gguf)
    pattern = f"*{args.quant}*.gguf"

    if args.source == "hf":
        if model_name in MODELS:
            print(f"Downloading {MODELS[model_name]} ({pattern}) from HuggingFace to {local_dir}...")
            snapshot_download(repo_id=MODELS[model_name], local_dir=local_dir, allow_patterns=[pattern])
            print("Done!")
        else:
            print(f"Model {model_name} not available on HuggingFace")
    elif args.source == "ms":
        if model_name in MS_MODELS:
            print(f"Downloading {MS_MODELS[model_name]} ({pattern}) from ModelScope to {local_dir}...")
            ms_snapshot_download(model_id=MS_MODELS[model_name], local_dir=local_dir, allow_file_pattern=[pattern])
            print("Done!")
        else:
            print(f"Model {model_name} not available on ModelScope")
