# Story vocabulary enrichment workflow

This workflow measures and improves dictionary coverage without deriving word
meanings from paragraph translations, overwriting editorial work, or treating
an API failure as missing data.

## 1. Generate the coverage report

Run the audit against the prepared story archive and the environment being
measured:

```powershell
go run ./cmd/story-vocab-audit `
  -archive="E:\leksuu\story-import-output\leksuu-stories-cleaned-draft.zip" `
  -api="https://leksuu.ir" `
  -out="reports\story-vocabulary-YYYY-MM-DD"
```

The command reads only `items/*.json` entries whose audience is `teen_adult`
and language is English. It writes:

- `coverage.json`: every normalized lemma, its forms, frequency, contexts and
  lookup result;
- `coverage.md`: completeness totals and review lists;
- `import-template.json`: missing fields only, blank and `verified: false`.

The public lookup API can measure completeness for the story corpus but cannot
enumerate unrelated vocabulary rows. For a full-table audit, export the English
rows through the authenticated `GET /api/v1/admin/vocab` endpoint and retain the
export with the review evidence.

## 2. Review meanings

Reviewers work on `import-template.json` in frequency order.

1. Confirm the lemma and the story contexts in `coverage.json`. Correct a
   heuristic lemma before translating it.
2. Treat articles, pronouns, auxiliaries, conjunctions and prepositions as real
   entries. Describe their grammatical function where a one-word gloss would
   be misleading.
3. Do not add ordinary definitions for people, places or other proper nouns.
   Add a record only when an editorial note is genuinely useful.
4. Validate Persian and Sorani independently with a qualified reviewer and a
   cited editorial source. Paragraph translations are context, not proof of a
   standalone dictionary meaning.
5. Fill only the locales listed in `needs`. Leave an already populated locale
   out of `meaning`.
6. Set `verified: true` only after both the lemma and supplied meanings have
   been reviewed. Keep `source` nonblank and specific to the review batch.

The generated file is not an assertion that its blank definitions are done.
Unverified or incomplete new entries are rejected by the importer.

## 3. Preview and import

Open **Admin → Vocabulary → Import reviewed vocabulary**, upload the reviewed
JSON and select **Preview import**. The preview classifies every row:

- `created`: a verified new entry with both Persian and Sorani meanings;
- `filled`: only blank fields on an existing entry will be populated;
- `unchanged`: the database already contains the supplied data;
- `needs_review`: unverified, incomplete or malformed;
- `conflict`: a supplied meaning differs from an existing nonblank meaning.

The apply button is disabled while conflicts exist. Applying skips unverified
and incomplete rows. The server uses the authenticated admin ID as reviewer and
never replaces a nonblank meaning, example or pronunciation.

For automation, send the same document to:

```http
POST /api/v1/admin/vocab/import
Authorization: Bearer <admin token>
Content-Type: application/json

{"dry_run":true,"items":[...]}
```

Repeat with `dry_run:false` only after checking the preview. Each request accepts
at most 3,000 rows and 4 MiB.

## 4. Re-measure

Run the audit again against the same archive and deployed environment. Compare
the two `coverage.json` summaries. Report definitions as complete only when both
`fa` and `ckb` are nonblank and the lookup completed successfully.
