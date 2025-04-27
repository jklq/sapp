package category

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
)

type JobResult struct {
	IsAmbiguityFlagged  bool        `json:"is_ambiguity_flagged"`
	AmbiguityFlagReason string      `json:"ambiguity_flag"`
	Spendings           []Spendings `json:"spendings"`
}

type Spendings struct {
	Id            int64   `json:"id"`
	Category      string  `json:"category"`
	Amount        float64 `json:"amount"`
	ApportionMode string  `json:"apportion_mode"`
	Description   string  `json:"description"`
}

type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	Refusal string `json:"refusal"`
}

type Choice struct {
	FinishReason string `json:"finish_reason"`
	Message      Message
}

type Person struct {
	Id   int64
	Name string
}

// SharedMode removed, AI infers apportionment from prompt
type CategorizationParams struct {
	TotalAmount float64
	Buyer       Person
	SharedWith  *Person // Potential partner, AI decides if used. Populated by handler.
	Prompt      string
	PreSettled  bool // Added: Flag to indicate if the job's spendings should be settled immediately
	tries       int
}

// ProcessCategorizationJob now requires the db connection and the ModelAPI implementation.
func ProcessCategorizationJob(db *sql.DB, api ModelAPI, params CategorizationParams) (JobResult, error) {
	if params.tries >= 3 {
		return JobResult{}, fmt.Errorf("too many retries")
	}
	params.tries += 1
	// Use the provided api instance instead of creating one here.

	// Pass the db connection to getPrompt
	prompt, err := getPrompt(db, params)
	if err != nil {
		return JobResult{}, err
	}

	res, err := api.Prompt(prompt)
	if err != nil {
		return JobResult{}, err
	}

	message := res.Choices[0].Message.Content

	jsonContent := []byte(message)
	job := JobResult{}

	slog.Info("llm generated text", "text", string(jsonContent))
	err = json.Unmarshal(jsonContent, &job)
	if err != nil {
		return JobResult{}, err
	}

	var countedTotal float64 = 0

	for _, spending := range job.Spendings {
		// Validate the ApportionMode provided by the AI for each spending item.
		isValidApportionMode := false
		validModes := []string{"alone", "shared", "other"}
		for _, mode := range validModes {
			if spending.ApportionMode == mode {
				isValidApportionMode = true
				break
			}
		}

		if !isValidApportionMode {
			slog.Warn("AI returned invalid apportion_mode, retrying", "spending_description", spending.Description, "invalid_mode", spending.ApportionMode)
			return ProcessCategorizationJob(db, api, params) // Retry, passing api
		}

		// If there's no shared person, the mode MUST be 'alone'.
		if params.SharedWith == nil && spending.ApportionMode != "alone" {
			slog.Warn("AI returned non-'alone' apportion_mode when there is no shared person, retrying", "spending_description", spending.Description, "mode", spending.ApportionMode)
			return ProcessCategorizationJob(db, api, params) // Retry, passing api
		}

		// Validation based on SharedMode hint is removed.
		// The AI now decides apportionment based solely on the prompt.
		// We still validate that 'other' is only used if a partner *exists*.
		if spending.ApportionMode == "other" && params.SharedWith == nil {
			slog.Warn("AI returned 'other' apportion_mode when there is no shared person configured, retrying", "spending_description", spending.Description)
			return ProcessCategorizationJob(db, api, params) // Retry, passing api
		}

		countedTotal += spending.Amount
	}

	// Check if the sum of amounts matches the total amount (within tolerance)
	// Use a small tolerance for floating point comparisons
	tolerance := 0.01 // e.g., 1 cent
	if math.Abs(countedTotal-params.TotalAmount) > tolerance {
		slog.Warn("spending amount and total amount did not match up, retrying", "counted_total", countedTotal, "actual_total", params.TotalAmount)
		return ProcessCategorizationJob(db, api, params) // Retry, passing api and db connection
	}

	if job.AmbiguityFlagReason != "" {
		job.IsAmbiguityFlagged = true
	}

	return job, nil
}
