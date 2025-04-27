package category

import (
	"database/sql"
	"fmt"
	"log/slog"
)

// SharedMode removed from Job struct
type Job struct {
	Id           int64 `json:"id"`
	TotalAmount  float64
	Status       string `json:"status"`
	IsFinished   bool   `json:"isFinished"`
	Prompt       string `json:"prompt"`
	Buyer        int64  `json:"buyer_id"`
	SharedWithId *int64 `json:"other_id"`
	PreSettled   bool   `json:"pre_settled"` // Added: Pre-settled flag from the job table
	Result       *JobResult
	Result       *JobResult
	SpendingDate *time.Time // Added: Store the specific spending date for the job
}

type CategorizingPoolStrategy interface {
	// AddJob adds a new categorization job with optional spending date.
	AddJob(params CategorizationParams, spendingDate *time.Time) (int64, error)
	StartPool()
	GetStatus(int64) (Job, error)
}

type CategorizingPool struct {
	db            *sql.DB
	numWorkers    int
	unhandledJobs chan Job
	api           ModelAPI
}

// NewCategorizingPool now accepts a ModelAPI implementation.
func NewCategorizingPool(db *sql.DB, numWorkers int, api ModelAPI) CategorizingPool {
	return CategorizingPool{
		db:            db,
		numWorkers:    numWorkers,
		unhandledJobs: make(chan Job, 100),
		api:           api, // Store the provided API
	}
}

// AddJob adds a new categorization job to the queue.
// It inserts the job details (including optional spending date) into the database
// and sends the job details to the channel for processing.
func (p CategorizingPool) AddJob(params CategorizationParams, spendingDate *time.Time) (int64, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var otherPersonInt *int64 = nil

	if params.SharedWith != nil {
		otherPersonInt = &params.SharedWith.Id
	}

	// Convert *time.Time to sql.NullTime for insertion
	var sqlSpendingDate sql.NullTime
	if spendingDate != nil {
		sqlSpendingDate = sql.NullTime{Time: *spendingDate, Valid: true}
	}

	// Store pre_settled flag and spending_date in the job record
	result, err := tx.Exec(`INSERT INTO ai_categorization_jobs (buyer, shared_with, prompt, total_amount, pre_settled, spending_date, status)
	VALUES (?, ?, ?, ?, ?, ?, ?)`, params.Buyer.Id, otherPersonInt, params.Prompt, params.TotalAmount, params.PreSettled, sqlSpendingDate, "pending")
	if err != nil {
		slog.Error("error inserting ai categorization job", "error", err, "pre_settled", params.PreSettled, "spending_date", spendingDate)
		return 0, err
	}

	jobId, err := result.LastInsertId()
	if err != nil {
		slog.Error("error getting ai categorization job ID", "error", err)
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Send job details to the worker channel (non-blocking send might be better if channel can fill)
	// We pass the full job details needed by the worker.
	job := Job{
		Id:           jobId,
		Params:       params,       // Pass the original params
		SpendingDate: spendingDate, // Pass the parsed spending date
		Status:       "pending",
		// Result is initially nil
	}
	// Consider adding a timeout or select with a default case for non-blocking send
	p.unhandledJobs <- job
	slog.Debug("Job sent to worker channel", "job_id", jobId)

	return jobId, nil
}

func (p CategorizingPool) GetStatus(id int64) (Job, error) {
	var jobStatus Job

	row := p.db.QueryRow("SELECT id, status, is_finished FROM ai_categorization_jobs WHERE id = ?", id)

	if err := row.Scan(&jobStatus.Id, &jobStatus.Status, &jobStatus.IsFinished); err != nil {
		return jobStatus, err
	}

	if !jobStatus.IsFinished {
		return jobStatus, nil
	}

	// TODO: Populate JobResult more completely if needed by the GetStatus consumer.
	// Currently, this function is not used by tested handlers.
	// Fetch spending_date as well if needed here.
	// row := p.db.QueryRow("SELECT id, status, is_finished, spending_date FROM ai_categorization_jobs WHERE id = ?", id)
	// var sqlSpendingDate sql.NullTime
	// ... scan &jobStatus.Id, &jobStatus.Status, &jobStatus.IsFinished, &sqlSpendingDate ...
	// if sqlSpendingDate.Valid { jobStatus.SpendingDate = &sqlSpendingDate.Time }


	if !jobStatus.IsFinished {
		return jobStatus, nil
	}

	// Example of fetching associated spendings if needed (adapt query as necessary)
	rows, err := p.db.Query(`SELECT s.id, s.amount, s.description -- Add other fields like category_name, spending_date if needed
	FROM spendings s
	INNER JOIN ai_categorized_spendings acs ON s.id = acs.spending_id
	WHERE acs.job_id = ?`, jobStatus.Id)

	if err != nil {
		return jobStatus, err
	}

	for rows.Next() {
		fmt.Println("one spending!")
		t := Spendings{}
		err := rows.Scan(&t.Id, &t.Amount, &t.Description)
		if err != nil {
			return jobStatus, err
		}
		jobStatus.Result.Spendings = append(jobStatus.Result.Spendings, t)
	}

	return jobStatus, nil
}
