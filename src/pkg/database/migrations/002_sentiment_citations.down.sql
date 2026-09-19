ALTER TABLE sentiment_scores
    DROP COLUMN IF EXISTS raw_response,
    DROP COLUMN IF EXISTS source_url;
