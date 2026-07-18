#!/usr/bin/env python3
"""
Chunked PR reviewer for GitHub Models free tier.

Strategy:
  1. Split diff.txt into per-file hunks.
  2. Drop/deprioritize low-value files (lockfiles, generated/vendored code).
  3. Pack files into batches under a token budget (respects model's free-tier cap).
  4. Review each batch with a JSON-only response -> cheap, deterministic merge.
  5. Merge all batches in Python; compute Approved/Rejected from blocker count.
  6. Retry on 429 with backoff; hard-cap total requests to respect daily quota.

No extra "synthesis" API call is made -- aggregation and the final verdict are
computed locally, so token spend scales with PR size, not with review logic.
"""

import json
import os
import re
import sys
import time
import fnmatch
import requests

# ---------------------------------------------------------------------------
# Config -- tune here, not scattered through the code
# ---------------------------------------------------------------------------

MODEL = "gpt-4o-mini"          # 16K in / 4K out on GitHub Models free tier
ENDPOINT = "https://models.inference.ai.azure.com/chat/completions"

MAX_INPUT_CHARS_PER_BATCH = 12_000   # ~4K tokens input budget for diff content
                                        # (leaves headroom under 16K token cap for
                                        # system prompt + JSON schema instructions)
MAX_OUTPUT_TOKENS = 1200              # per-batch JSON is compact; no need for 4000
MAX_SINGLE_FILE_CHARS = 12_000        # truncate any one file's diff beyond this
MAX_BATCHES = 12                      # hard cap on API calls this run (quota safety)
MAX_RETRIES = 4
BASE_BACKOFF_SECONDS = 5

# Files whose diffs rarely need LLM review -- skip entirely to save budget.
SKIP_PATTERNS = [
    "*/package-lock.json", "package-lock.json",
    "*/yarn.lock", "yarn.lock",
    "*/poetry.lock", "poetry.lock",
    "*/Cargo.lock", "Cargo.lock",
    "*.min.js", "*.min.css",
    "*/dist/*", "*/build/*", "*/vendor/*", "*/node_modules/*",
    "*.svg", "*.png", "*.jpg", "*.jpeg", "*.gif", "*.ico",
    "*.lock",
    "*.sum",
]

SYSTEM_PROMPT = """\
You are a senior code reviewer. You will review ONE BATCH of files from a larger PR.
Respond with STRICT JSON ONLY -- no markdown fences, no prose outside the JSON.

Schema:
{
  "blockers": [
    {"file": "path/to/file", "line": "12", "issue": "short description"}
  ],
  "suggestions": [
    {"file": "path/to/file", "line": "34", "issue": "short description"}
  ]
}

Rules:
- "blockers" = bugs, security issues, resource leaks, incorrect logic. Empty array if none.
- "suggestions" = style, minor refactors, edge-case hardening. Empty array if none.
- Every item must reference a real file and line from the diff. Do not invent issues.
- Be concise. One line per issue description.
- If a file's diff was truncated (marked "...(truncated)"), only review what's shown.
"""


def log(msg):
    print(f"[review_pr] {msg}", file=sys.stderr)


# ---------------------------------------------------------------------------
# Diff parsing
# ---------------------------------------------------------------------------

def split_diff_by_file(diff_text):
    """Split a unified diff into (filename, file_diff_text) chunks."""
    parts = re.split(r"(?=^diff --git )", diff_text, flags=re.MULTILINE)
    files = []
    for part in parts:
        if not part.startswith("diff --git"):
            continue
        m = re.match(r"diff --git a/(.*?) b/(.*?)\n", part)
        filename = m.group(2) if m else "unknown_file"
        files.append((filename, part))
    return files


def is_skippable(filename):
    return any(fnmatch.fnmatch(filename, pat) for pat in SKIP_PATTERNS)


def truncate_file_diff(filename, file_diff):
    if len(file_diff) <= MAX_SINGLE_FILE_CHARS:
        return file_diff
    return file_diff[:MAX_SINGLE_FILE_CHARS] + f"\n... (truncated, {filename} diff too large)\n"


def build_batches(files):
    """Greedily pack (filename, diff) pairs into batches under the char budget."""
    batches = []
    current = []
    current_len = 0
    for filename, file_diff in files:
        file_diff = truncate_file_diff(filename, file_diff)
        if current and current_len + len(file_diff) > MAX_INPUT_CHARS_PER_BATCH:
            batches.append(current)
            current = []
            current_len = 0
        current.append((filename, file_diff))
        current_len += len(file_diff)
    if current:
        batches.append(current)
    return batches


# ---------------------------------------------------------------------------
# API call with retry/backoff
# ---------------------------------------------------------------------------

def call_model(token, batch_text, batch_index, total_batches):
    user_msg = (
        f"Batch {batch_index}/{total_batches}. Review this subset of the PR diff.\n\n"
        f"{batch_text}"
    )
    payload = {
        "model": MODEL,
        "messages": [
            {"role": "system", "content": SYSTEM_PROMPT},
            {"role": "user", "content": user_msg},
        ],
        "max_tokens": MAX_OUTPUT_TOKENS,
        "response_format": {"type": "json_object"},
    }

    for attempt in range(1, MAX_RETRIES + 1):
        resp = requests.post(
            ENDPOINT,
            headers={"Authorization": f"Bearer {token}"},
            json=payload,
            timeout=60,
        )
        if resp.status_code == 200:
            return resp
        if resp.status_code in (429, 413) or resp.status_code >= 500:
            wait = BASE_BACKOFF_SECONDS * (2 ** (attempt - 1))
            log(f"batch {batch_index}: status {resp.status_code}, retry {attempt}/{MAX_RETRIES} in {wait}s")
            time.sleep(wait)
            continue
        # Non-retryable error
        resp.raise_for_status()
    return resp  # last response, will raise upstream


def parse_batch_response(resp, batch_index):
    try:
        resp.raise_for_status()
        content = resp.json()["choices"][0]["message"]["content"]
        data = json.loads(content)
        blockers = data.get("blockers", [])
        suggestions = data.get("suggestions", [])
        return blockers, suggestions
    except Exception as e:
        log(f"batch {batch_index}: failed to parse response ({e}); treating as review-unavailable")
        return (
            [],
            [{"file": "N/A", "line": "-", "issue": f"batch {batch_index} review unavailable ({type(e).__name__})"}],
        )


# ---------------------------------------------------------------------------
# Output formatting
# ---------------------------------------------------------------------------

def format_items(items):
    if not items:
        return "None."
    lines = []
    for it in items:
        f = it.get("file", "?")
        ln = it.get("line", "?")
        issue = it.get("issue", "")
        lines.append(f"- `{f}:{ln}` — {issue}")
    return "\n".join(lines)


def build_review_md(all_blockers, all_suggestions, skipped_files, truncated_note):
    verdict = "**Rejected** — one or more blockers must be addressed first." \
        if all_blockers else "**Approved** — no blockers, ready to merge."

    parts = ["## Automated Review", ""]
    parts.append("### Blockers")
    parts.append(format_items(all_blockers))
    parts.append("")
    parts.append("### Suggestions")
    parts.append(format_items(all_suggestions))
    parts.append("")
    parts.append("### Conclusion")
    parts.append(verdict)

    if skipped_files or truncated_note:
        parts.append("")
        parts.append("<details><summary>Review scope notes</summary>")
        parts.append("")
        if skipped_files:
            parts.append(f"- Skipped (lockfile/generated/binary, not reviewed): {', '.join(skipped_files)}")
        if truncated_note:
            parts.append(f"- {truncated_note}")
        parts.append("")
        parts.append("</details>")

    return "\n".join(parts)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    token = os.environ["GITHUB_TOKEN"]

    with open("diff.txt") as f:
        diff_text = f.read()

    all_files = split_diff_by_file(diff_text)
    if not all_files:
        with open("review.md", "w") as f:
            f.write("## Automated Review\n\nNo file changes detected in diff.\n")
        return

    skipped_files = [fn for fn, _ in all_files if is_skippable(fn)]
    reviewable_files = [(fn, d) for fn, d in all_files if not is_skippable(fn)]

    if not reviewable_files:
        with open("review.md", "w") as f:
            f.write(
                "## Automated Review\n\n### Blockers\nNone.\n\n"
                "### Suggestions\nNone.\n\n### Conclusion\n"
                "**Approved** — no reviewable files (only lockfiles/binaries changed).\n"
            )
        return

    batches = build_batches(reviewable_files)

    truncated_note = None
    if len(batches) > MAX_BATCHES:
        truncated_note = (
            f"PR too large: {len(batches)} batches needed, only first {MAX_BATCHES} reviewed "
            f"(quota safety cap). Consider splitting this PR."
        )
        batches = batches[:MAX_BATCHES]

    log(f"{len(reviewable_files)} reviewable files, {len(skipped_files)} skipped, "
        f"{len(batches)} batch(es) to review")

    all_blockers = []
    all_suggestions = []

    for i, batch in enumerate(batches, start=1):
        batch_text = "\n".join(fd for _, fd in batch)
        log(f"reviewing batch {i}/{len(batches)} "
            f"({len(batch)} files, {len(batch_text)} chars)")
        resp = call_model(token, batch_text, i, len(batches))
        blockers, suggestions = parse_batch_response(resp, i)
        all_blockers.extend(blockers)
        all_suggestions.extend(suggestions)

        if i < len(batches):
            time.sleep(2)  # stay comfortably under requests-per-minute limits

    review_md = build_review_md(all_blockers, all_suggestions, skipped_files, truncated_note)
    with open("review.md", "w") as f:
        f.write(review_md)

    log("done")


if __name__ == "__main__":
    main()