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

ALTER TABLE companies
    ADD COLUMN IF NOT EXISTS teamtailor_feed_url text,
    ADD COLUMN IF NOT EXISTS teamtailor_checked_careers_url text;

CREATE TABLE IF NOT EXISTS jobs (
    id bigserial PRIMARY KEY,
    company_id bigint NOT NULL REFERENCES companies (id),
    guid text NOT NULL,
    title text NOT NULL,
    url text NOT NULL,
    remote text NOT NULL DEFAULT '',
    open_application boolean NOT NULL,
    published_at timestamptz,
    first_seen_at timestamptz NOT NULL,
    last_seen_at timestamptz NOT NULL,
    closed_at timestamptz,
    UNIQUE (company_id, guid)
);

CREATE TABLE IF NOT EXISTS job_places (
    job_id bigint NOT NULL REFERENCES jobs (id),
    city text NOT NULL,
    country text NOT NULL,
    PRIMARY KEY (job_id, city, country)
);

CREATE TABLE IF NOT EXISTS job_locations (
    job_id bigint NOT NULL REFERENCES jobs (id),
    location_id bigint NOT NULL REFERENCES locations (id),
    PRIMARY KEY (job_id, location_id)
);
