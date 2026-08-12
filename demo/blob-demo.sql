-- creel blob demo — a tiny SQLite schema for testing binary cell rendering.
--
-- Build:
--   sqlite3 demo/blob-demo.db < demo/blob-demo.sql
-- Explore:
--   ./creel -database demo/blob-demo.db
--
-- Then open the `files` table (s), move onto a `data` cell, and try:
--   E              view-only blob summary
--   :saveblob ~/Desktop/out.bin
--   Y              copy row as INSERT (should emit X'…' literals)

PRAGMA foreign_keys = ON;

DROP TABLE IF EXISTS files;
DROP TABLE IF EXISTS notes;

CREATE TABLE files (
    id          INTEGER PRIMARY KEY,
    name        TEXT    NOT NULL,
    mime        TEXT,
    description TEXT,
    data        BLOB,          -- the column under test
    thumb       BLOB,          -- second blob col (often NULL)
    size_bytes  INTEGER,       -- denormalized size for easy eyeballing
    created_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE notes (
    id      INTEGER PRIMARY KEY,
    title   TEXT NOT NULL,
    body    TEXT               -- plain text control: should NOT become <BLOB …>
);

-- 1×1 transparent PNG (68 bytes) — saveblob should produce a real image file.
INSERT INTO files (id, name, mime, description, data, thumb, size_bytes) VALUES (
    1,
    'pixel.png',
    'image/png',
    'tiny valid PNG — :saveblob then open it',
    X'89504e470d0a1a0a0000000d49484452000000010000000108060000001f15c4890000000a49444154789c63000100000500010d0a2db40000000049454e44ae426082',
    NULL,
    67
);

-- ~1.2KB of zeros — placeholder should read "<BLOB 1.2KB>".
INSERT INTO files (id, name, mime, description, data, thumb, size_bytes) VALUES (
    2,
    'padding.bin',
    'application/octet-stream',
    'zeroblob(1234) — checks KB formatting',
    zeroblob(1234),
    NULL,
    1234
);

-- Empty non-NULL blob — "<BLOB 0B>", distinct from NULL.
INSERT INTO files (id, name, mime, description, data, thumb, size_bytes) VALUES (
    3,
    'empty.bin',
    'application/octet-stream',
    'X'''' empty blob — not the same as NULL',
    X'',
    NULL,
    0
);

-- SQL NULL — muted "NULL" sentinel, no Blobs entry.
INSERT INTO files (id, name, mime, description, data, thumb, size_bytes) VALUES (
    4,
    'missing.bin',
    NULL,
    'data IS NULL',
    NULL,
    NULL,
    NULL
);

-- Mixed printable + high bytes (includes NUL) — was garbage before the fix.
INSERT INTO files (id, name, mime, description, data, thumb, size_bytes) VALUES (
    5,
    'mixed.bin',
    'application/octet-stream',
    'NUL + text + 0xFF — used to corrupt the grid',
    X'48656c6c6f0000ff776f726c64',  -- Hello\0\0\xffworld
    X'deadbeef',
    13
);

-- Larger blob so you can see MB-ish sizing without a huge dump (~1.5MB).
INSERT INTO files (id, name, mime, description, data, thumb, size_bytes) VALUES (
    6,
    'big.bin',
    'application/octet-stream',
    'zeroblob(1572864) — "<BLOB 1.5MB>"',
    zeroblob(1572864),
    NULL,
    1572864
);

-- Control table: TEXT only, so you can confirm normal columns are untouched.
INSERT INTO notes (id, title, body) VALUES
    (1, 'welcome', 'Plain text — must never render as a BLOB placeholder.'),
    (2, 'empty body', ''),
    (3, 'null body', NULL);
