import openai
import sys

# Connect to the local Forge server
client = openai.OpenAI(
    base_url="http://localhost:8080/v1",
    api_key="not-needed-for-mock"
)

def test_model(model_name):
    print(f"--- Testing {model_name} ---")
    try:
        response = client.chat.completions.create(
            model=model_name,
            messages=[{"role": "user", "content": "Hello!"}],
            stream=True
        )

        print("Response: ", end="")
        for chunk in response:
            if chunk.choices and chunk.choices[0].delta.content:
                print(chunk.choices[0].delta.content, end="")
        print("\n" + "-"*30)
    except Exception as e:
        print(f"Error testing {model_name}: {e}")

if __name__ == "__main__":
    models = ["qwen", "llama", "minimax", "oss"]

    # Try fetching models list first
    try:
        print("Fetching models list...")
        avail_models = client.models.list()
        print("Available models:")
        for m in avail_models.data:
            print(f"- {m.id} (provider: {m.provider})")
    except Exception as e:
        print(f"Could not fetch models: {e}")

    for m in models:
        test_model(m)
