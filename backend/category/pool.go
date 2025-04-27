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
}

type CategorizingPoolStrategy interface {
	AddJob(CategorizationParams) (int64, error)
	StartPool()
	GetStatus(int64) (Job, error)
}

type CategorizingPool struct {
	db            *sql.DB
	numWorkers    int
	unhandledJobs chan Job
	api           ModelAPI // Add ModelAPI dependency
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

func (p CategorizingPool) AddJob(params CategorizationParams) (int64, error) {
	tx, err := p.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var otherPersonInt *int64 = nil

	if params.SharedWith != nil {
		otherPersonInt = &params.SharedWith.Id
	}

	// Added pre_settled to INSERT statement
	result, err := tx.Exec(`INSERT INTO ai_categorization_jobs (buyer, shared_with, prompt, total_amount, pre_settled)
	VALUES (?, ?, ?, ?, ?)`, params.Buyer.Id, otherPersonInt, params.Prompt, params.TotalAmount, params.PreSettled)
	if err != nil {
		slog.Error("error inserting ai categorization job", "error", err, "pre_settled", params.PreSettled)
		return 0, err
	}

	jobId, err := result.LastInsertId()
	if err != nil {
		slog.Error("error getting ai categorization job ID", "error", err)
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
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

	// TODO: Populate JobResult more completely by joining user_spendings as well
	// to determine sharing status etc., if needed by the GetStatus consumer.
	// Currently, this function is not used by tested handlers.

	rows, err := p.db.Query(`SELECT s.id, s.amount, s.description
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
