# Forge

Forge is a unified AI backend and frontend delivered as a single static binary written in Go. It abstracts LLM inference, manages stateful conversation context, and provides a secure sandbox for tool execution.

## Features

- **Zero-dependency deployment**: Single static binary, SQLite embedded, frontend embedded.
- **Inspector UI**: See exactly what the model sees. Token counts, raw context, tool execution traces.
- **Built-in tool execution**: First-class Pause-Execute-Resume tool loop with sandboxing.
- **OpenAI-compatible API**: `/v1/chat/completions` and `/v1/models` endpoints out of the box.
- **Multi-provider support**: Ollama, OpenAI, Anthropic, Gemini, Qwen, Minimax, OSS, etc.

## Setup

1. Build the Forge server:
   ```bash
   go build -o forge ./cmd/forge
   ```

2. Run the server:
   ```bash
   ./forge
   ```

3. Configure via environment variables:
   - `FORGE_ADDR`: Server address (default: `:8080`)
   - `OPENAI_API_KEY`: OpenAI API key
   - `QWEN_API_KEY`: Qwen API key
   - `LLAMA_API_KEY`: Llama API key
   - `MINIMAX_API_KEY`: Minimax API key
   - `OSS_API_KEY`: OSS API key

## Testing

A python script `test_endpoints.py` is included to test against mock providers for Qwen, Llama, Minimax, and OSS.

Install dependencies:
```bash
pip install -r requirements.txt
```

Run test script:
```bash
python test_endpoints.py
```

## Downloading Models

A script `download_models.py` is included to easily download local models via HuggingFace Hub or ModelScope:

```bash
python download_models.py --model qwen --source hf
```
