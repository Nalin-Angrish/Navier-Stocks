import os
import requests

token = os.environ["GITHUB_TOKEN"]
endpoint = "https://models.inference.ai.azure.com/chat/completions"

with open("diff.txt") as f:
    diff = f.read()

MAX_CHARS = 100_000
if len(diff) > MAX_CHARS:
    diff = diff[:MAX_CHARS] + "\n... (truncated)"

SYSTEM_PROMPT = """\
You are a senior code reviewer. Your review must follow this exact template:

## Automated Review

### Blockers
Problems that MUST be fixed before merging. Include file/line references.
Significance: actual bugs, security vulnerabilities, resource leaks, incorrect logic.
If none, write "None."

### Suggestions
Optional improvements. Style, best practices, minor refactors, edge-case hardening.
If none, write "None."

### Conclusion
**Approved** — no blockers, ready to merge.
**Rejected** — one or more blockers must be addressed first.

Be concise. Every item must reference a specific file and line. Do not invent issues.
"""

resp = requests.post(
    endpoint,
    headers={"Authorization": f"Bearer {token}"},
    json={
        "model": "Llama-3.3-70B-Instruct",
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": f"Review this PR diff.\n\n{diff}"}
        ],
        "max_tokens": 4000,
    },
)
resp.raise_for_status()
text = resp.json()["choices"][0]["message"]["content"]

with open("review.md", "w") as f:
    f.write(text)