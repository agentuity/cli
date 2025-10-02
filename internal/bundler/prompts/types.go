package prompts

// Prompt represents a single prompt definition from YAML
type Prompt struct {
	Name        string   `yaml:"name"`
	Slug        string   `yaml:"slug"`
	Description string   `yaml:"description"`
	System      string   `yaml:"system"`
	Prompt      string   `yaml:"prompt"`
	Evals       []string `yaml:"evals,omitempty"`

	// Parsed template information
	SystemTemplate Template `json:"system_template,omitempty"`
	PromptTemplate Template `json:"prompt_template,omitempty"`
}
