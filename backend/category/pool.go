package category

import (
	"database/sql"
	"fmt"
	"log/slog"
)

type Job struct {
	Id           int64 `json:"id"`
	TotalAmount  float64
	Status       string `json:"status"`
	SharedMode   string `json:"shared_mode"`
	IsFinished   bool   `json:"isFinished"`
	Prompt       string `json:"prompt"`
	Buyer        int64  `json:"buyer_id"`
	SharedWithId *int64 `json:"other_id"`
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
}

func NewCategorizingPool(db *sql.DB, numWorkers int) CategorizingPool {
	return CategorizingPool{db: db, numWorkers: numWorkers, unhandledJobs: make(chan Job, 100)}
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

	result, err := tx.Exec(`INSERT INTO ai_categorization_jobs (buyer, shared_with, prompt, total_amount, shared_mode) 
	VALUES (?, ?, ?, ?, ?)`, params.Buyer.Id, otherPersonInt, params.Prompt, params.TotalAmount, params.SharedMode)
	if err != nil {
		slog.Error("error inserting ai categorization job", "error", err)
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

	// TODO: legg til JobStatus. da må en joine ai_categorized_spendings og user_spendings for å finne
	// riktig spendings og hvilke status de har (alene, mix eller other)

	rows, err := p.db.Query(`SELECT id, amount, description
	FROM spendings INNER JOIN ai_categorized_spendings 
	AT spendings.id = ai_categorized_spendings.spending_id 
	WHERE ai_categorized_spendings.job_id = ?`, jobStatus.Id)

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
