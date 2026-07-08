-- Granular per-entity access control: pin a subscription requirement to any
-- course, unit, or lesson. Absence of a row means "inherit" from the parent.
CREATE TABLE IF NOT EXISTS access_rules (
    entity_type text NOT NULL,           -- 'course' | 'unit' | 'lesson'
    entity_id   text NOT NULL,
    requirement text NOT NULL,           -- 'free' | 'courses' | 'classes'
    PRIMARY KEY (entity_type, entity_id)
);
