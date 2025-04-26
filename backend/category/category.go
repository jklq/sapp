package category

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"os"
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

type CategorizationParams struct {
	DB          *sql.DB
	TotalAmount float64
	SharedMode  string
	Buyer       Person
	SharedWith  *Person
	Prompt      string
	tries       int
}

func ProcessCategorizationJob(params CategorizationParams) (JobResult, error) {
	if params.tries >= 3 {
		return JobResult{}, fmt.Errorf("too many retries")
	}
	params.tries += 1

	var api ModelAPI = NewOpenRouterAPI(os.Getenv("OPENROUTER_KEY"), "deepseek/deepseek-chat") //

	prompt, err := getPrompt(params)
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

	for i, t := range job.Spendings {
		if params.SharedMode == "shared" {
			job.Spendings[i].ApportionMode = "shared"
		} else if params.SharedMode == "alone" {
			job.Spendings[i].ApportionMode = "alone"
		} else if params.SharedMode == "mix" && (t.ApportionMode != "alone" && t.ApportionMode != "shared" && t.ApportionMode != "other") {
			slog.Warn("generated sharedStatus was not correct")
			return ProcessCategorizationJob(params)
		} else if params.SharedMode != "mix" {
			slog.Warn("invalid sharedStatus")
			return JobResult{}, fmt.Errorf("invalid shared status")
		}

		countedTotal += t.Amount
	}

	if math.Abs(countedTotal-params.TotalAmount) > 3 {
		slog.Warn("spending amount and total amount did not match up", "counted_total", countedTotal, "actual_total", params.TotalAmount)
		return ProcessCategorizationJob(params)
	}

	if job.AmbiguityFlagReason != "" {
		job.IsAmbiguityFlagged = true
	}

	return job, nil
}
