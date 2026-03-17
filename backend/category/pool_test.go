package category

import (
	"database/sql"
	"os"
	"testing"

	_ "modernc.org/sqlite"
)

func TestRequeueBackfillJobs(t *testing.T) {
	db := setupPoolTestDB(t)
	defer db.Close()

	var buyerID int64
	if err := db.QueryRow("SELECT id FROM users WHERE username = 'demo_user'").Scan(&buyerID); err != nil {
		t.Fatalf("querying buyer ID: %v", err)
	}

	var partnerID int64
	if err := db.QueryRow("SELECT id FROM users WHERE username = 'partner_user'").Scan(&partnerID); err != nil {
		t.Fatalf("querying partner ID: %v", err)
	}

	pool := NewCategorizingPool(db, 1, OpenRouterAPI{})

	failedJobID := insertAIJobForTest(t, db, buyerID, &partnerID, "failed prompt", 75, "failed", true)
	pendingJobID := insertAIJobForTest(t, db, buyerID, &partnerID, "pending prompt", 50, "pending", false)
	completedJobID := insertAIJobForTest(t, db, buyerID, &partnerID, "completed prompt", 25, "completed", true)

	var categoryID int64
	if err := db.QueryRow("SELECT id FROM categories WHERE name = 'Groceries'").Scan(&categoryID); err != nil {
		t.Fatalf("querying category ID: %v", err)
	}
	insertCategorizedSpendingForTest(t, db, buyerID, &partnerID, categoryID, completedJobID)

	requeued, err := pool.RequeueBackfillJobs()
	if err != nil {
		t.Fatalf("RequeueBackfillJobs() error = %v", err)
	}
	if requeued != 2 {
		t.Fatalf("RequeueBackfillJobs() requeued = %d, expected 2", requeued)
	}

	requeuedIDs := map[int64]bool{}
	for i := 0; i < requeued; i++ {
		job := <-pool.unhandledJobs
		requeuedIDs[job.Id] = true
		if job.Status != "pending" {
			t.Fatalf("requeued job status = %q, expected pending", job.Status)
		}
	}

	if !requeuedIDs[failedJobID] {
		t.Fatalf("failed job %d was not requeued", failedJobID)
	}
	if !requeuedIDs[pendingJobID] {
		t.Fatalf("pending job %d was not requeued", pendingJobID)
	}
	if requeuedIDs[completedJobID] {
		t.Fatalf("completed job %d should not have been requeued", completedJobID)
	}

	var status string
	var isFinished bool
	var errorMessage sql.NullString
	err = db.QueryRow("SELECT status, is_finished, error_message FROM ai_categorization_jobs WHERE id = ?", failedJobID).Scan(&status, &isFinished, &errorMessage)
	if err != nil {
		t.Fatalf("querying failed job after requeue: %v", err)
	}
	if status != "pending" {
		t.Fatalf("failed job status after requeue = %q, expected pending", status)
	}
	if isFinished {
		t.Fatalf("failed job should have been marked unfinished")
	}
	if errorMessage.Valid {
		t.Fatalf("failed job error_message should have been cleared, got %q", errorMessage.String)
	}
}

func setupPoolTestDB(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", "file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("opening in-memory database: %v", err)
	}

	schema, err := os.ReadFile("../cmd/migrate/schema.sql")
	if err != nil {
		db.Close()
		t.Fatalf("reading schema: %v", err)
	}

	if _, err := db.Exec(string(schema)); err != nil {
		db.Close()
		t.Fatalf("running schema: %v", err)
	}

	return db
}

func insertAIJobForTest(t *testing.T, db *sql.DB, buyerID int64, sharedWithID *int64, prompt string, totalAmount float64, status string, isFinished bool) int64 {
	t.Helper()

	res, err := db.Exec(`
		INSERT INTO ai_categorization_jobs
			(buyer, shared_with, prompt, total_amount, status, is_finished, error_message)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, buyerID, sharedWithID, prompt, totalAmount, status, isFinished, "old error")
	if err != nil {
		t.Fatalf("inserting AI job: %v", err)
	}

	jobID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("reading AI job ID: %v", err)
	}

	return jobID
}

func insertCategorizedSpendingForTest(t *testing.T, db *sql.DB, buyerID int64, sharedWithID *int64, categoryID int64, jobID int64) {
	t.Helper()

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer tx.Rollback()

	res, err := tx.Exec(`
		INSERT INTO spendings (amount, description, category, made_by)
		VALUES (?, ?, ?, ?)
	`, 25.0, "already categorized", categoryID, buyerID)
	if err != nil {
		t.Fatalf("inserting spending: %v", err)
	}

	spendingID, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("reading spending ID: %v", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO user_spendings (spending_id, buyer, shared_with, shared_user_takes_all)
		VALUES (?, ?, ?, ?)
	`, spendingID, buyerID, sharedWithID, false); err != nil {
		t.Fatalf("inserting user_spending: %v", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO ai_categorized_spendings (spending_id, job_id)
		VALUES (?, ?)
	`, spendingID, jobID); err != nil {
		t.Fatalf("linking categorized spending: %v", err)
	}

	if err := tx.Commit(); err != nil {
		t.Fatalf("committing categorized spending transaction: %v", err)
	}
}
