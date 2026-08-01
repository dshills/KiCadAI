package opentopologysynthesis

const (
	CausalRepairSchema  = "kicadai.simulation-guided-causal-repair.v1"
	CausalRepairVersion = 1
)

type CausalRepairBudget struct {
	Trials               int `json:"trials"`
	ValueTrials          int `json:"value_trials"`
	TopologyTrials       int `json:"topology_trials"`
	CoordinatedTrials    int `json:"coordinated_trials"`
	MaximumChanges       int `json:"maximum_changes"`
	CandidateSimulations int `json:"candidate_simulations"`
	CornerEvaluations    int `json:"corner_evaluations"`
}

type CausalRepairConsumption struct {
	Trials               int  `json:"trials"`
	ValueTrials          int  `json:"value_trials"`
	TopologyTrials       int  `json:"topology_trials"`
	CoordinatedTrials    int  `json:"coordinated_trials"`
	CandidateSimulations int  `json:"candidate_simulations"`
	CornerEvaluations    int  `json:"corner_evaluations"`
	BudgetExhausted      bool `json:"budget_exhausted"`
}

type CausalPerturbation struct {
	Kind             string   `json:"kind"`
	InstanceID       string   `json:"instance_id,omitempty"`
	Terminal         string   `json:"terminal,omitempty"`
	FromNode         string   `json:"from_node,omitempty"`
	ToNode           string   `json:"to_node,omitempty"`
	FromPrimitiveKey string   `json:"from_primitive_key,omitempty"`
	ToPrimitiveKey   string   `json:"to_primitive_key,omitempty"`
	FromValue        *float64 `json:"from_value,omitempty"`
	ToValue          *float64 `json:"to_value,omitempty"`
	Magnitude        float64  `json:"magnitude"`
	Hash             string   `json:"hash"`
}

type CausalAssertionEffect struct {
	RequirementID     string  `json:"requirement_id"`
	OperatingCase     string  `json:"operating_case"`
	CornerID          string  `json:"corner_id"`
	Analysis          string  `json:"analysis"`
	Metric            string  `json:"metric"`
	Critical          bool    `json:"critical"`
	BaselinePass      bool    `json:"baseline_pass"`
	TrialPass         bool    `json:"trial_pass"`
	BaselineViolation float64 `json:"baseline_violation"`
	TrialViolation    float64 `json:"trial_violation"`
	ViolationDelta    float64 `json:"violation_delta"`
	BaselineMargin    float64 `json:"baseline_margin"`
	TrialMargin       float64 `json:"trial_margin"`
	MarginDelta       float64 `json:"margin_delta"`
	Sensitivity       float64 `json:"sensitivity"`
	Regression        bool    `json:"regression"`
	Reason            string  `json:"reason,omitempty"`
}

type CausalRepairTrial struct {
	Number            int                        `json:"number"`
	Perturbations     []CausalPerturbation       `json:"perturbations"`
	GraphHash         string                     `json:"graph_hash"`
	EvaluationHash    string                     `json:"evaluation_hash"`
	Status            SimulationEvaluationStatus `json:"status"`
	Effects           []CausalAssertionEffect    `json:"effects"`
	BaselineViolation float64                    `json:"baseline_violation"`
	TrialViolation    float64                    `json:"trial_violation"`
	Improvement       float64                    `json:"improvement"`
	Sensitivity       float64                    `json:"sensitivity"`
	ChangeMagnitude   float64                    `json:"change_magnitude"`
	Regressions       []string                   `json:"regressions"`
	Authorized        bool                       `json:"authorized"`
	Rejection         string                     `json:"rejection,omitempty"`
	Coordinated       bool                       `json:"coordinated"`
	Rank              int                        `json:"rank,omitempty"`
	Repair            Repair                     `json:"repair"`
	Evaluation        SimulationEvaluation       `json:"evaluation"`
	Hash              string                     `json:"hash"`
}

type CausalRepairAnalysis struct {
	Schema                string                  `json:"schema"`
	Version               int                     `json:"version"`
	PolicyVersion         string                  `json:"policy_version"`
	RequirementHash       string                  `json:"requirement_hash"`
	InventoryHash         string                  `json:"inventory_hash"`
	InitialGraphHash      string                  `json:"initial_graph_hash"`
	InitialEvaluationHash string                  `json:"initial_evaluation_hash"`
	Budget                CausalRepairBudget      `json:"budget"`
	Consumption           CausalRepairConsumption `json:"consumption"`
	Trials                []CausalRepairTrial     `json:"trials"`
	SelectedTrialHash     string                  `json:"selected_trial_hash,omitempty"`
	Status                string                  `json:"status"`
	Hash                  string                  `json:"hash"`
}
