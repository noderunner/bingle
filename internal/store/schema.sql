CREATE TABLE IF NOT EXISTS games (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    room_code     TEXT NOT NULL UNIQUE,
    game_def_id   TEXT NOT NULL,
    status        TEXT NOT NULL DEFAULT 'waiting', -- waiting | active | finished
    board_state   TEXT NOT NULL,                   -- JSON snapshot, current
    turn_seat     INTEGER,                          -- NULL while waiting/finished
    move_seq      INTEGER NOT NULL DEFAULT 0,
    winner_seat   INTEGER,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS players (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id       INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seat          INTEGER NOT NULL,
    name          TEXT NOT NULL,
    color         TEXT NOT NULL,
    joined_at     TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_id, seat),
    UNIQUE(game_id, name COLLATE NOCASE)
);

CREATE TABLE IF NOT EXISTS moves (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id              INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seq                  INTEGER NOT NULL,
    seat                 INTEGER NOT NULL,
    color                TEXT NOT NULL,
    from_position        TEXT NOT NULL,
    to_position          TEXT NOT NULL,
    captured_position    TEXT,
    resulting_state      TEXT NOT NULL, -- JSON snapshot AFTER this move, for O(1) replay-at-seq
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(game_id, seq)
);

CREATE INDEX IF NOT EXISTS idx_moves_game_seq ON moves(game_id, seq);

-- A person's choice, per game and seat, of when to be asked about their
-- reactions (the rest are declined for them; see session.ReactionSettings):
-- JSON {"v":2,"targeted":bool,"ask_all":bool,"ask_seats":[...]}. Rows from
-- before the choice was reversed hold {"all":bool,"seats":[...]} instead.
CREATE TABLE IF NOT EXISTS reaction_prefs (
    game_id       INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seat          INTEGER NOT NULL,
    prefs         TEXT NOT NULL,
    PRIMARY KEY (game_id, seat)
);

-- The table talk of a game: players (seat > 0) and spectators (seat 0).
CREATE TABLE IF NOT EXISTS chat_messages (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    game_id       INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seat          INTEGER NOT NULL DEFAULT 0,
    name          TEXT NOT NULL,
    body          TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_chat_game_id ON chat_messages(game_id, id);

-- The global chat room of the home page: everybody sees it, no game needed.
CREATE TABLE IF NOT EXISTS global_chat (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    name          TEXT NOT NULL,
    body          TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- A person's Auto-Play Timeout, per game and seat (see session.AutoPlay):
-- when it is their turn to act and they have not for timeout_seconds, a
-- tactical bot plays for them. Off by default, 24 hours when on.
CREATE TABLE IF NOT EXISTS autoplay_prefs (
    game_id         INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    seat            INTEGER NOT NULL,
    enabled         INTEGER NOT NULL DEFAULT 0,
    timeout_seconds INTEGER NOT NULL DEFAULT 86400,
    PRIMARY KEY (game_id, seat)
);

-- Bot practice batches (see internal/batch): many simulated games with the
-- metrics of each. A batch is its spec plus a base seed; its games are
-- reproducible from them. Nothing here belongs to a room.
CREATE TABLE IF NOT EXISTS batches (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL DEFAULT '',
    game_def_id TEXT NOT NULL,
    spec        TEXT NOT NULL,
    status      TEXT NOT NULL DEFAULT 'queued', -- queued | running | done | cancelled | failed | interrupted
    n_games     INTEGER NOT NULL,
    done        INTEGER NOT NULL DEFAULT 0,
    error       TEXT NOT NULL DEFAULT '',
    created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    started_at  TIMESTAMP,
    finished_at TIMESTAMP
);

-- metrics is a cthulhuwars.GameMetrics document; events (only the first
-- games of a batch keep them) a list of decisions.
CREATE TABLE IF NOT EXISTS batch_games (
    batch_id   INTEGER NOT NULL REFERENCES batches(id) ON DELETE CASCADE,
    idx        INTEGER NOT NULL,
    seed       INTEGER NOT NULL,
    game_id    TEXT NOT NULL,
    seating    TEXT NOT NULL,
    winner     INTEGER NOT NULL DEFAULT 0,
    end_reason TEXT NOT NULL DEFAULT '',
    plies      INTEGER NOT NULL DEFAULT 0,
    rounds     INTEGER NOT NULL DEFAULT 0,
    metrics    TEXT NOT NULL,
    events     TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (batch_id, idx)
);

-- Email notifications (see session/notify.go and docs/history/email-notifications-plan.md).
-- A secret that proves a seat belongs to the browser that took it (only a hash
-- is kept), keyed by the player's name in the game.
-- Seat tokens proved a seat for the notify endpoints until seats were owned by
-- a principal (ADR 0067); the table is gone.
DROP TABLE IF EXISTS seat_tokens;

-- An address a browser asked to be notified at. It is mailed only once
-- verified (verified_at set by the confirmation link). secret_hash proves the
-- browser owns the contact; stop_token is the one-click unsubscribe secret.
-- The address itself is never returned by any endpoint.
CREATE TABLE IF NOT EXISTS notify_contacts (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    address          TEXT NOT NULL,
    addr_hash        TEXT NOT NULL,
    secret_hash      TEXT NOT NULL,
    stop_token       TEXT NOT NULL,
    confirm_hash     TEXT NOT NULL DEFAULT '',
    confirm_expires  TIMESTAMP,
    verified_at      TIMESTAMP,
    created_at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_notify_contacts_confirm ON notify_contacts(confirm_hash);
CREATE INDEX IF NOT EXISTS idx_notify_contacts_stop ON notify_contacts(stop_token);

-- Whether a seat is to be mailed when it is its turn, and where.
-- away_minutes 0 = every time; N = only if the turn waited N minutes.
CREATE TABLE IF NOT EXISTS notify_prefs (
    game_id       INTEGER NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    name_key      TEXT NOT NULL,
    contact_id    INTEGER REFERENCES notify_contacts(id) ON DELETE CASCADE,
    enabled       INTEGER NOT NULL DEFAULT 0,
    away_minutes  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (game_id, name_key)
);

-- Addresses that asked never to be mailed again (hashes only).
CREATE TABLE IF NOT EXISTS notify_suppress (
    addr_hash     TEXT PRIMARY KEY,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Every mail sent, for the rate limits (they survive a restart).
CREATE TABLE IF NOT EXISTS notify_sent (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    kind          TEXT NOT NULL,
    addr_hash     TEXT NOT NULL DEFAULT '',
    game_id       INTEGER NOT NULL DEFAULT 0,
    ip            TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_notify_sent_at ON notify_sent(at);

-- The versions each game implementation has had on this database: one row the
-- first time the server starts with a version (the version of a game is in its
-- definition and goes up whenever the definition or the Go code behind it changes).
CREATE TABLE IF NOT EXISTS game_versions (
    family      TEXT NOT NULL,
    version     TEXT NOT NULL,
    first_seen  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (family, version)
);

-- Whoever acts: a guest, or (later) an account (ADR 0067). The ID is the
-- server's own name for them and is never shown; a guest proves it is one with
-- a key, below. merged_into is set when a guest's key was adopted by another
-- device's guest and its seats moved there.
CREATE TABLE IF NOT EXISTS principals (
    id            TEXT PRIMARY KEY,
    kind          TEXT NOT NULL,
    account_id    INTEGER,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    merged_into   TEXT NOT NULL DEFAULT ''
);

-- A guest's keys, as SHA-256 hashes: whoever holds a key is that guest.
CREATE TABLE IF NOT EXISTS guest_keys (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    principal_id  TEXT NOT NULL REFERENCES principals(id),
    key_hash      TEXT NOT NULL UNIQUE,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    revoked_at    TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_guest_keys_principal ON guest_keys(principal_id);

-- Accounts (the accounts plan, stage 3). A handle is unique case-blind and
-- shown as @handle; status is "pending" until the address is confirmed,
-- then "active". principal_id is the account's principal, which may have
-- been the guest that signed up (ADR 0068).
CREATE TABLE IF NOT EXISTS accounts (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    handle               TEXT NOT NULL,
    handle_lower         TEXT NOT NULL UNIQUE,
    display_name         TEXT NOT NULL,
    email                TEXT NOT NULL,
    email_lower          TEXT NOT NULL,
    email_verified_at    TIMESTAMP,
    status               TEXT NOT NULL,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    principal_id         TEXT NOT NULL,
    created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at           TIMESTAMP
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_email ON accounts(email_lower) WHERE deleted_at IS NULL;

-- How an account proves itself: a password now; TOTP, recovery codes and
-- providers later. secret is the encoded hash, params whatever the kind needs.
CREATE TABLE IF NOT EXISTS credentials (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id    INTEGER NOT NULL REFERENCES accounts(id),
    kind          TEXT NOT NULL,
    secret        TEXT NOT NULL,
    params        TEXT NOT NULL DEFAULT '',
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at  TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_credentials_account ON credentials(account_id, kind);

CREATE TABLE IF NOT EXISTS groups (
    name          TEXT PRIMARY KEY
);

INSERT OR IGNORE INTO groups (name) VALUES ('admin');

CREATE TABLE IF NOT EXISTS account_groups (
    account_id    INTEGER NOT NULL REFERENCES accounts(id),
    group_name    TEXT NOT NULL REFERENCES groups(name),
    PRIMARY KEY (account_id, group_name)
);

-- Signed-in browsers, by the SHA-256 of the cookie's value. Signing out is a
-- delete, and so is signing out everywhere.
CREATE TABLE IF NOT EXISTS sessions (
    id_hash       TEXT PRIMARY KEY,
    account_id    INTEGER NOT NULL REFERENCES accounts(id),
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at    TIMESTAMP NOT NULL,
    ip_hash       TEXT NOT NULL DEFAULT '',
    agent         TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_sessions_account ON sessions(account_id);

-- One-time links mailed to an account (the confirmation of its address), by
-- the SHA-256 of the token.
CREATE TABLE IF NOT EXISTS account_tokens (
    hash          TEXT PRIMARY KEY,
    account_id    INTEGER NOT NULL REFERENCES accounts(id),
    kind          TEXT NOT NULL,
    expires_at    TIMESTAMP NOT NULL,
    used_at       TIMESTAMP
);

-- What happened to whom: sign-ins and their failures, signups, password
-- changes, the seeded administrator.
CREATE TABLE IF NOT EXISTS auth_events (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    account_id    INTEGER,
    kind          TEXT NOT NULL,
    detail        TEXT NOT NULL DEFAULT '',
    ip_hash       TEXT NOT NULL DEFAULT ''
);

-- A provider's identity is one account's at most.
CREATE UNIQUE INDEX IF NOT EXISTS idx_credentials_oidc ON credentials(secret) WHERE kind = 'oidc';

-- What each class may do (stage 6 of the accounts plan, ADR 0071), seeded
-- from bingle-capabilities.yaml on a fresh database and managed on the
-- admin page from then on. The admin group may do everything and has no rows.
CREATE TABLE IF NOT EXISTS capabilities (
    class         TEXT NOT NULL,
    capability    TEXT NOT NULL,
    allowed       INTEGER NOT NULL,
    PRIMARY KEY (class, capability)
);

-- Per-class rate limits, per minute: chat lines, games made, direct messages.
CREATE TABLE IF NOT EXISTS cap_limits (
    class         TEXT NOT NULL,
    kind          TEXT NOT NULL,
    per_minute    INTEGER NOT NULL,
    PRIMARY KEY (class, kind)
);

-- API tokens: an account's, for scripts and agents, by the SHA-256 of the
-- token. scopes is a comma-separated list; a token never carries the admin
-- group's powers.
CREATE TABLE IF NOT EXISTS api_tokens (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id    INTEGER NOT NULL REFERENCES accounts(id),
    name          TEXT NOT NULL,
    hash          TEXT NOT NULL UNIQUE,
    scopes        TEXT NOT NULL,
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_used_at  TIMESTAMP,
    revoked_at    TIMESTAMP
);

-- Invitations to sign up, when BINGLE_SIGNUP=invite: by the SHA-256 of the
-- code in the link, used once.
CREATE TABLE IF NOT EXISTS invites (
    code_hash     TEXT PRIMARY KEY,
    created_by    INTEGER NOT NULL REFERENCES accounts(id),
    created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at    TIMESTAMP NOT NULL,
    used_by       INTEGER,
    used_at       TIMESTAMP
);
