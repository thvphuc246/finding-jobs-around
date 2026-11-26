CREATE TABLE IF NOT EXISTS companies (
    id bigint PRIMARY KEY,
    name text NOT NULL,
    website_url text,
    inactive_at timestamptz,
    updated_at timestamptz NOT NULL
);

CREATE TABLE IF NOT EXISTS locations (
    id bigint PRIMARY KEY,
    company_id bigint NOT NULL REFERENCES companies (id),
    kind text NOT NULL CHECK (kind IN ('head_office', 'branch')),
    street text NOT NULL,
    postal_code text NOT NULL,
    city text NOT NULL,
    lat double precision,
    lon double precision,
    approximate boolean NOT NULL DEFAULT false,
    geocoded_address text,
    inactive_at timestamptz,
    updated_at timestamptz NOT NULL,
    CHECK ((lat IS NULL) = (lon IS NULL))
);

ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS careers_url text,
    ADD COLUMN IF NOT EXISTS careers_source text CHECK (careers_source IN ('csv', 'detected')),
    ADD COLUMN IF NOT EXISTS careers_checked_website text;
