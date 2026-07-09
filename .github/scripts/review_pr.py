import os
import requests

token = os.environ["GITHUB_TOKEN"]
endpoint = "https://models.inference.ai.azure.com/chat/completions"

with open("diff.txt") as f:
    diff = f.read()

MAX_CHARS = 100_000
if len(diff) > MAX_CHARS:
    diff = diff[:MAX_CHARS] + "\n... (truncated)"

resp = requests.post(
    endpoint,
    headers={"Authorization": f"Bearer {token}"},
    json={
        "model": "Llama-3.3-70B-Instruct",  # or "Mistral-large", "Phi-4"
        "messages": [
            {"role": "system", "content": "You are a senior code reviewer."},
            {"role": "user", "content": f"Review this PR diff. Flag bugs, security issues, and style problems. Be concise, markdown format, reference file/line where possible.\n\n{diff}"}
        ],
        "max_tokens": 4000,
    },
)
resp.raise_for_status()
text = resp.json()["choices"][0]["message"]["content"]

with open("review.md", "w") as f:
    f.write(f"## Automated Review (GitHub Models)\n\n{text}")