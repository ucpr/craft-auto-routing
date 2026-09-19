package typesafe

// ChoiceOption describes one candidate answer for a "choice" question.
type ChoiceOption struct {
	Description string `json:"description,omitempty"`
}

type choiceQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type systemOneRequest struct {
	State     any                       `json:"state"`
	Model     string                    `json:"model"`
	Questions map[string]choiceQuestion `json:"questions"`
}

type systemOneResponse struct {
	Model   string                  `json:"model"`
	Answers map[string]choiceAnswer `json:"answers"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type choiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// ClassifyResult is the outcome of a single choice classification.
type ClassifyResult struct {
	Choice        string
	Confidence    float64
	Probabilities map[string]float64
}
