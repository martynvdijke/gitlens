package migrate

import (
	"context"
	"fmt"
	"log"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

// DedupeRepositories keeps only the row with the most recent
// synced_at per (user_repositories, github_id) pair, deleting the
// rest. This is a one-shot migration step required before adding the
// composite unique index (user, github_id) to the Repository schema;
// it must run before client.Schema.Create.
//
// Safe to call repeatedly: once the duplicates are gone, the
// per-pair MIN(synced_at) == MAX(synced_at) and the DELETE matches
// zero rows.
//
// Designed for SQLite (the only dialect GitLens supports today). For
// PostgreSQL/MySQL the syntax would need adaptation.
func DedupeRepositories(ctx context.Context, drv *entsql.Driver) error {
	if drv == nil {
		return fmt.Errorf("nil driver")
	}

	// 1. Count duplicates so the log message is meaningful.
	rows, err := drv.DB().QueryContext(ctx, `
		SELECT user_repositories, github_id, COUNT(*) as c
		FROM repositories
		GROUP BY user_repositories, github_id
		HAVING c > 1
	`)
	if err != nil {
		return fmt.Errorf("counting duplicates: %w", err)
	}
	var dupes int
	for rows.Next() {
		var userID, repoID int64
		var c int
		if err := rows.Scan(&userID, &repoID, &c); err != nil {
			rows.Close()
			return fmt.Errorf("scanning duplicate row: %w", err)
		}
		dupes += c - 1
	}
	rows.Close()

	if dupes == 0 {
		log.Println("DedupeRepositories: no duplicate repositories found")
		return nil
	}
	log.Printf("DedupeRepositories: found %d duplicate repository rows, deduplicating...", dupes)

	// 2. Delete duplicates, keeping the row with the latest
	//    synced_at (NULLs sort last in DESC).
	if _, err := drv.DB().ExecContext(ctx, `
		DELETE FROM repositories
		WHERE id NOT IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (
					PARTITION BY user_repositories, github_id
					ORDER BY synced_at DESC NULLS LAST, id DESC
				) AS rn
				FROM repositories
			) WHERE rn = 1
		)
	`); err != nil {
		return fmt.Errorf("deleting duplicate repositories: %w", err)
	}

	log.Printf("DedupeRepositories: removed %d duplicate rows", dupes)
	return nil
}

// Ensure dialect import is referenced; some builds drop it.
var _ = dialect.SQLite
