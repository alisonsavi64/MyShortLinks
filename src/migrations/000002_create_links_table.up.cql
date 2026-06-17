CREATE TABLE IF NOT EXISTS myshortlinks.links (
    short_code text PRIMARY KEY,
    original_url text,
    created_at timestamp
);
