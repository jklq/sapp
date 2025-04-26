package category

import (
	"fmt"
	"sync"
)

// MockModelAPI is a mock implementation of the ModelAPI interface for testing.
type MockModelAPI struct {
	mu             sync.Mutex
	PromptFunc     func(prompt string) (*ModelAPIResponse, error)
	PromptHistory  []string // Records the prompts received
	ResponseQueue  []*ModelAPIResponse // Predefined responses to return
	ErrorQueue     []error             // Predefined errors to return
	DefaultResponse *ModelAPIResponse   // Response if queue is empty
	DefaultError    error               // Error if queue is empty
}

// NewMockModelAPI creates a new mock API instance.
func NewMockModelAPI() *MockModelAPI {
	return &MockModelAPI{
		PromptHistory: []string{},
		ResponseQueue: []*ModelAPIResponse{},
		ErrorQueue:    []error{},
	}
}

// Prompt implements the ModelAPI interface for the mock.
func (m *MockModelAPI) Prompt(prompt string) (*ModelAPIResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.PromptHistory = append(m.PromptHistory, prompt)

	// Use predefined function if set
	if m.PromptFunc != nil {
		return m.PromptFunc(prompt)
	}

	// Return queued error if available
	if len(m.ErrorQueue) > 0 {
		err := m.ErrorQueue[0]
		m.ErrorQueue = m.ErrorQueue[1:] // Dequeue
		return nil, err
	}

	// Return queued response if available
	if len(m.ResponseQueue) > 0 {
		resp := m.ResponseQueue[0]
		m.ResponseQueue = m.ResponseQueue[1:] // Dequeue
		return resp, nil
	}

	// Return default error if set
	if m.DefaultError != nil {
		return nil, m.DefaultError
	}

	// Return default response if set
	if m.DefaultResponse != nil {
		return m.DefaultResponse, nil
	}

	// Default behavior if nothing is configured
	return nil, fmt.Errorf("mock Prompt called but no response or error was configured")
}

// AddResponse adds a response to the queue.
func (m *MockModelAPI) AddResponse(response *ModelAPIResponse) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResponseQueue = append(m.ResponseQueue, response)
}

// AddError adds an error to the queue.
func (m *MockModelAPI) AddError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ErrorQueue = append(m.ErrorQueue, err)
}

// ClearHistory resets the prompt history.
func (m *MockModelAPI) ClearHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PromptHistory = []string{}
}

// ClearQueues resets the response and error queues.
func (m *MockModelAPI) ClearQueues() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ResponseQueue = []*ModelAPIResponse{}
	m.ErrorQueue = []*error{}
}
