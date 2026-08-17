package server

type compareRequest struct {
	RunA       string             `json:"run_a"`
	RunB       string             `json:"run_b"`
	Conditions []featureCondition `json:"conditions"`
}

type featureCondition struct {
	Feature  string   `json:"feature"`
	Operator string   `json:"operator"`
	Value    *float64 `json:"value"`
}

type visualizerDownloadRequest struct {
	URL string `json:"url"`
}

type inputGeneratorOption struct {
	Name   string `json:"name"`
	Dir    string `json:"dir"`
	Source string `json:"source"`
}

type inputParameter struct {
	Name       string `json:"name"`
	Constraint string `json:"constraint"`
}

type toolInputGenerateRequest struct {
	GeneratorDir   string           `json:"generator_dir"`
	Generator      string           `json:"generator"`
	CaseCount      int              `json:"case_count"`
	SeedStart      uint64           `json:"seed_start"`
	InputDirectory string           `json:"input_directory"`
	Overwrite      bool             `json:"overwrite"`
	Parameters     []inputParameter `json:"parameters"`
}

type toolInputGenerateResponse struct {
	OutputDir string `json:"output_dir"`
	CaseCount int    `json:"case_count"`
}

type inputFeatureDefinition struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Line   int    `json:"line,omitempty"`
	Column int    `json:"column,omitempty"`
	Source string `json:"source,omitempty"`
}

type featureCaseValues struct {
	InputCaseID string             `json:"input_case_id"`
	Values      map[string]float64 `json:"values"`
}

type featureExtractionIssue struct {
	InputCaseID string `json:"input_case_id"`
	Feature     string `json:"feature,omitempty"`
	Message     string `json:"message"`
}

type featureDataResponse struct {
	Features   []inputFeatureDefinition `json:"features"`
	Cases      []featureCaseValues      `json:"cases"`
	IssueCount int                      `json:"issue_count"`
	Issues     []featureExtractionIssue `json:"issues"`
}
