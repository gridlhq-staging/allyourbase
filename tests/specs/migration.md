# Migration Tools Test Specification (Tier 2)

**Purpose:** Detailed test cases for migrating from PocketBase, Supabase, and Firebase to AYB (BDD Tier 1: B-MIG-001 through B-MIG-004)

**Related BDD Stories:**
- [B-MIG-001: Migrate from PocketBase](../../docs/reference/bdd/migration.md#b-mig-001-migrate-from-pocketbase)
- [B-MIG-002: Migrate from Supabase](../../docs/reference/bdd/migration.md#b-mig-002-migrate-from-supabase)
- [B-MIG-003: Migrate from Firebase](../../docs/reference/bdd/migration.md#b-mig-003-migrate-from-firebase)
- [B-MIG-004: Auto-Detect Migration Source](../../docs/reference/bdd/migration.md#b-mig-004-auto-detect-migration-source)

---

## PocketBase Migration Tests

### TC-MIG-PB-001: Migrate PocketBase — Happy Path

**Story:** B-MIG-001
**Type:** CLI + Integration
**Fixture:** `tests/fixtures/migration/pocketbase/happy-path/`

**Fixture Structure:**
```
pocketbase/happy-path/
├── pb_data/
│   └── data.db (SQLite with users, collections, settings)
└── expected/
    ├── users.json (expected ayb_users records)
    ├── posts.json (expected posts records)
    └── summary.json (expected migration summary)
```

**Command:**
```bash
ayb migrate pocketbase --source ./tests/fixtures/migration/pocketbase/happy-path/pb_data --database-url $DB_URL
```

**Expected Output:**
```
✓ Analyzing PocketBase data...
  - Found 2 auth users
  - Found 1 collection: posts (5 records)

✓ Phase 1: Migrating schema...
  - Created table: posts

✓ Phase 2: Migrating auth users...
  - Migrated 2 users (bcrypt hashes preserved)

✓ Phase 3: Migrating collection data...
  - Migrated posts: 5 records

✓ Migration complete!
  - Users: 2
  - Collections: 1 (5 total records)
```

**Assertions:**
- Command exits with status 0
- `ayb_users` table contains 2 users with preserved IDs
- Password hashes are bcrypt format (`$2a$`)
- `posts` table exists with 5 records
- Foreign keys (author_id) reference correct user IDs
- Created/updated timestamps preserved

---

### TC-MIG-PB-002: Migrate PocketBase — Idempotent Re-Run

**Story:** B-MIG-001
**Type:** CLI + Integration
**Fixture:** `tests/fixtures/migration/pocketbase/idempotent/`

**Steps:**
1. Run migration command first time
2. Verify data migrated successfully
3. Run same migration command again
4. Verify no duplicate records created

**Expected Behavior:**
- First run: all data migrated
- Second run: detects existing data, skips duplicates
- Final record counts match expected (no duplicates)

**Assertions:**
- `ayb_users` has exactly 2 records (not 4)
- `posts` has exactly 5 records (not 10)
- Idempotency via primary key constraints or ON CONFLICT

---

### TC-MIG-PB-003: Migrate PocketBase — Empty Database

**Story:** B-MIG-001
**Type:** CLI
**Fixture:** `tests/fixtures/migration/pocketbase/empty/`

**Command:**
```bash
ayb migrate pocketbase --source ./tests/fixtures/migration/pocketbase/empty/pb_data --database-url $DB_URL
```

**Expected Output:**
```
✓ Analyzing PocketBase data...
  - Found 0 auth users
  - Found 0 collections

⚠ Nothing to migrate (empty database)
```

**Assertions:**
- Command exits with status 0
- No errors thrown
- Graceful handling of empty source

---

### TC-MIG-PB-004: Migrate PocketBase — Invalid Path

**Story:** B-MIG-001
**Type:** CLI

**Command:**
```bash
ayb migrate pocketbase --source ./non_existent_path --database-url $DB_URL
```

**Expected Output:**
```
Error: PocketBase data directory not found: ./non_existent_path
```

**Assertions:**
- Command exits with non-zero status
- Clear error message about missing directory

---

## Supabase Migration Tests

### TC-MIG-SB-001: Migrate Supabase — Full Migration (5 Phases)

**Story:** B-MIG-002
**Type:** CLI + Integration
**Fixture:** `tests/fixtures/migration/supabase/full/`

**Fixture Structure:**
```
supabase/full/
├── source_dump.sql (PostgreSQL dump with schema, data, RLS)
└── expected/
    ├── users.json (expected ayb_users records)
    ├── oauth_accounts.json (expected ayb_oauth_accounts)
    ├── rls_policies.json (expected ayb_rls_policies)
    └── summary.json
```

**Command:**
```bash
ayb migrate supabase --source-url $SUPABASE_DB_URL --database-url $AYB_DB_URL
```

**Expected Output:**
```
✓ Analyzing Supabase database...
  - Tables: 3 (users, posts, comments)
  - Auth users: 10
  - OAuth accounts: 5
  - RLS policies: 8

✓ Phase 1: Migrating schema...
  - Created table: posts (3 columns, 1 FK)
  - Created table: comments (4 columns, 2 FKs)

✓ Phase 2: Migrating data...
  - posts: 50 records
  - comments: 120 records

✓ Phase 3: Migrating auth users...
  - Migrated 10 users (bcrypt hashes preserved)

✓ Phase 4: Migrating OAuth accounts...
  - Migrated 5 OAuth accounts (3 Google, 2 GitHub)

✓ Phase 5: Migrating RLS policies...
  - Migrated 8 RLS policies

✓ Migration complete!
  - Tables: 2
  - Records: 170
  - Users: 10
  - OAuth accounts: 5
  - RLS policies: 8
```

**Assertions:**
- All 5 phases complete successfully
- Schema, data, auth, OAuth, RLS all migrated
- Bcrypt password hashes preserved
- RLS policies active (can be queried via `ayb_rls_policies`)

---

### TC-MIG-SB-002: Migrate Supabase — ON CONFLICT Handling

**Story:** B-MIG-002
**Type:** CLI + Integration

**Steps:**
1. Run migration with 10 posts
2. Re-run migration with 15 posts (5 new, 10 existing)
3. Verify final count is 15 (not 25)

**Assertions:**
- ON CONFLICT clauses prevent duplicates
- Existing records preserved
- New records added
- No primary key violations

---

### TC-MIG-SB-003: Migrate Supabase — Invalid Source URL

**Story:** B-MIG-002
**Type:** CLI

**Command:**
```bash
ayb migrate supabase --source-url postgres://invalid:5432/db --database-url $AYB_DB_URL
```

**Expected Output:**
```
Error: Failed to connect to Supabase database: connection refused
```

**Assertions:**
- Command exits with non-zero status
- Clear error about connection failure

---

> Retirement note: Firebase migration coverage is historical in-tree support only (retired from roadmap scope). These cases document implemented behavior and do not schedule new Firebase roadmap work.

## Firebase Migration Tests

### TC-MIG-FB-001: Migrate Firebase — Auth + Firestore

**Story:** B-MIG-003
**Type:** CLI + Integration
**Fixture:** `tests/fixtures/migration/firebase/full/`

**Fixture Structure:**
```
firebase/full/
├── auth-export.json (auth users with firebase-scrypt hashes)
├── firestore-export/
│   ├── posts.json
│   └── comments.json
└── expected/
    ├── users.json (expected ayb_users with firebase-scrypt hashes)
    ├── posts.json (expected posts with JSONB metadata)
    └── summary.json
```

**Command:**
```bash
ayb migrate firebase \
  --auth-export ./tests/fixtures/migration/firebase/full/auth-export.json \
  --firestore-export ./tests/fixtures/migration/firebase/full/firestore-export \
  --database-url $DB_URL
```

**Expected Output:**
```
✓ Analyzing Firebase exports...
  - Auth users: 8
  - Firestore collections: 2 (posts, comments)

✓ Phase 1: Migrating auth users...
  - Migrated 8 users (firebase-scrypt hashes preserved)

✓ Phase 2: Migrating Firestore collections...
  - posts: 30 documents → 30 records
  - comments: 75 documents → 75 records

✓ Migration complete!
  - Users: 8
  - Collections: 2 (105 total records)
```

**Assertions:**
- Firebase scrypt password hashes stored as `$firebase-scrypt$...`
- Firestore nested JSON → JSONB columns
- Document IDs preserved
- Timestamps converted to PostgreSQL format

---

### TC-MIG-FB-002: Migrate Firebase — Scrypt Parameters Embedded

**Story:** B-MIG-003
**Type:** Integration
**Fixture:** `tests/fixtures/migration/firebase/scrypt/`

**Setup:**
Firebase export with user:
```json
{
  "users": [
    {
      "localId": "user123",
      "email": "test@example.com",
      "passwordHash": "base64hash",
      "salt": "base64salt",
      "createdAt": "1609459200000"
    }
  ],
  "config": {
    "signerKey": "base64key",
    "saltSeparator": "base64sep",
    "rounds": 8,
    "memCost": 14
  }
}
```

**Expected `ayb_users` record:**
```sql
-- "user123" is not a valid UUID, so FirebaseIDToUUID converts it
-- to a deterministic UUID v5 using a fixed namespace.
INSERT INTO ayb_users (id, email, password_hash) VALUES (
  '<deterministic-uuid-v5-of-user123>',
  'test@example.com',
  '$firebase-scrypt$<signerKey>$<saltSep>$<salt>$8$14$<hash>'
);
```

**Assertions:**
- All scrypt parameters embedded in password hash
- Progressive re-hashing works on first login (bcrypt/firebase → argon2id)

---

### TC-MIG-FB-003: Migrate Firebase — Full Export Inputs (Auth + Firestore + RTDB + Storage)

**Story:** B-MIG-003
**Type:** CLI
**Fixture:** `tests/fixtures/migration/firebase/full/` + storage fixture directory

**Command:**
```bash
ayb migrate firebase \
  --auth-export ./tests/fixtures/migration/firebase/full/auth-export.json \
  --firestore-export ./tests/fixtures/migration/firebase/full/firestore-export \
  --rtdb-export ./tests/fixtures/migration/firebase/full/rtdb-export.json \
  --storage-export ./tests/fixtures/migration/firebase/full/storage-export \
  --database-url $DB_URL
```

**Expected Behavior:**
- Uses offline export inputs only (no `firebase://` source flow in this command)
- Runs the same pre-flight analysis + confirmation pattern as other migration commands
- Migrates all provided source categories in one run

**Assertions:**
- Auth users are migrated with Firebase scrypt hash preservation
- Firestore/RTDB exports are mapped into JSONB-backed tables
- Storage files are copied to AYB storage layout
- Migration completes successfully

---

### TC-MIG-FB-004: Migrate Firebase — Malformed Auth Export JSON

**Story:** B-MIG-003
**Type:** CLI

**Command:**
```bash
ayb migrate firebase --auth-export ./invalid.json --database-url $DB_URL
```

**File Contents:**
```json
{
  invalid json
}
```

**Expected Output:**
```
Error: analysis failed: parsing auth export: parsing auth export JSON: ...
```

**Assertions:**
- Command exits with non-zero status
- Clear error about malformed auth export JSON
- Structurally unexpected but valid JSON is not currently documented as a missing-field validation path

---

## Auto-Detection Tests

### TC-MIG-AUTO-001: Auto-Detect PocketBase

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from ./tests/fixtures/migration/pocketbase/happy-path/pb_data
```

**Expected Behavior:**
- Detects PocketBase from directory structure (contains `data.db`)
- Auto-selects `pbmigrate.Migrate()`
- Migration proceeds as PocketBase migration

**Assertions:**
- Correct migrator selected
- No manual `--type` flag required

---

### TC-MIG-AUTO-002: Auto-Detect Supabase

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from postgres://user:pass@db.supabase.co:5432/postgres
```

**Expected Behavior:**
- Detects Supabase from connection string (contains `.supabase.` hostname)
- Auto-selects `sbmigrate.Migrate()`

**Assertions:**
- Correct migrator selected

---

### TC-MIG-AUTO-003: Auto-Detect Firebase (JSON)

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from ./tests/fixtures/migration/firebase/full/auth-export.json
```

**Expected Behavior:**
- Detects Firebase from `.json` file suffix
- Auto-selects Firebase migration path and passes the file as auth export input

**Assertions:**
- Correct migrator selected

---

### TC-MIG-AUTO-004: Auto-Detect Firebase (URL)

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from firebase://my-project-id
```

**Expected Behavior:**
- Detects Firebase from `firebase://` scheme
- Returns error: `firebase --from requires a path to a .json auth export file`

**Assertions:**
- Detection occurs, but auto-migration does not proceed without a `.json` auth export path

---

### TC-MIG-AUTO-005: Auto-Detect Unknown Source

**Story:** B-MIG-004
**Type:** CLI

**Command:**
```bash
ayb start --from ./random_directory
```

**Expected Output:**
```
Error: could not detect migration source type from "./random_directory" (expected: path to pb_data, postgres:// URL, or firebase:// URL)
```

**Assertions:**
- Command exits with non-zero status
- Clear error message for unsupported source type
- Current HEAD message lists PocketBase, Supabase, and `firebase://` URL hints; `.json` auth-export auto-detection is covered separately in TC-MIG-AUTO-003

---

## Browser Test Coverage (Unmocked)

### Implemented Browser Tests

None (migration is CLI-only).

### Missing Browser Tests

N/A — Migration tools are CLI-only, no UI tests needed.

---

## Fixture Requirements

### PocketBase Fixtures
1. `tests/fixtures/migration/pocketbase/happy-path/` — Full PocketBase migration
2. `tests/fixtures/migration/pocketbase/idempotent/` — Re-run test
3. `tests/fixtures/migration/pocketbase/empty/` — Empty database

### Supabase Fixtures
4. `tests/fixtures/migration/supabase/full/` — 5-phase migration
5. `tests/fixtures/migration/supabase/schema-only/` — Schema without data
6. `tests/fixtures/migration/supabase/rls-policies/` — RLS policy migration

### Firebase Fixtures (historical in-tree coverage)
7. `tests/fixtures/migration/firebase/full/` — Auth + Firestore (historical in-tree coverage)
8. `tests/fixtures/migration/firebase/scrypt/` — Scrypt parameter embedding (historical in-tree coverage)
9. `tests/fixtures/migration/firebase/auth-only/` — Auth without Firestore (historical in-tree coverage)

---

## Test Tags

- `migration`
- `pocketbase`
- `supabase`
- `firebase`
- `cli`
- `idempotency`
- `password-hashing`
- `rls`

---

**Last Updated:** Session 021 (2026-03-31)
