-- Adds citation metadata to sentiment_scores so every stored score can be
-- traced back to the originating article (source_url) and the exact LLM
-- output that produced it (raw_response).  Story 4.3 — Database persistence.
ALTER TABLE sentiment_scores
    ADD COLUMN IF NOT EXISTS source_url TEXT,
    ADD COLUMN IF NOT EXISTS raw_response TEXT;
