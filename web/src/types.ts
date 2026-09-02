// Types mirroring the Go evidence objects.
//
// These are deliberately a transcription of the server's schema rather than a
// convenient reshaping of it. The receipt is the audit artifact; a dashboard
// that restructures it on the way in creates a second place for a number to
// be wrong, and the whole point of the object is that there is exactly one.

export type Status =
  | "VERIFIED"
  | "AMBIGUOUS"
  | "UNDERDETERMINED"
  | "NARROWING_SENSITIVE"
  | "UNRESOLVED";

export const STATUSES: Status[] = [
  "VERIFIED",
  "AMBIGUOUS",
  "UNDERDETERMINED",
  "NARROWING_SENSITIVE",
  "UNRESOLVED",
];

export interface Remediation {
  action: string;
  effect: string;
  projected_collision_index?: number;
  projected_pool_n?: number;
}

export interface Uniqueness {
  method: string;
  scope: string;
  scope_source: "feasibility_gate" | "declared_txn_count";
  scope_note?: string;
  k_max_if_gate_derived?: number;
  scope_complete: boolean;
  matches_found: number;
  rivals_found: number;
  counted_after_dedup: boolean;
  count_saturated: boolean;
  alternative_witnesses: string[][];
  cumulative_index_in_searched_region: number;
}

export interface Miss {
  nearest_sum_paise: number;
  gap_paise: number;
  cardinality: number;
  valid: boolean;
}

export interface SolverBlock {
  method: string;
  k_max: number;
  k_max_source: string;
  split: [number, number];
  entries_left: number;
  entries_right: number;
  entry_encoding: string;
  memory_bytes: number;
  memory_ceiling_bytes: number;
  probed_targets: string[];
  solve_side: string;
  dedup_applied: boolean;
  dedup_removed: number;
  nearest_miss?: Miss;
}

export interface FeasibilityPoint {
  k: number;
  subsets: number;
  collision_index: number;
  collision_index_analytic: number;
}

export interface Feasibility {
  n: number;
  contribution_sigma_paise: number;
  lattice_gcd_paise: number;
  k_star: number;
  collision_index_at_k_star: number;
  collision_index_estimator: string;
  collision_index_analytic_at_k_star: number;
  cumulative_index_at_k_star: number;
  threshold_underdetermined: number;
  lattice_correction_applied: boolean;
  decision: string;
  declared_txn_count?: number;
  implied_free_cardinality?: number;
  collision_index_at_implied?: number;
  predicted_entries: number;
  predicted_bytes: number;
  memory_ceiling_bytes: number;
  curve: FeasibilityPoint[];
  note: string;
}

export interface TwinClass {
  value_paise: number;
  members: number[];
}

export interface Entropy {
  distinct_contribution_values: number;
  twin_classes?: TwinClass[];
  twin_class_count: number;
  twin_mass: number;
  twin_mass_threshold: number;
  lattice_gcd_paise: number;
  zero_contribution_members?: number[];
  pass: boolean;
  note?: string;
}

export interface Check {
  name: string;
  state: "pass" | "fail" | "inactive";
  detail: string;
}

export interface Substitution {
  removed: string[];
  added: string[];
  depth: number;
  sum_delta_paise: number;
}

export interface Neighbourhood {
  method: string;
  stable: boolean;
  max_substitution_depth: number;
  requested_substitution_depth: number;
  removal_sums_enumerated: number;
  addition_sums_enumerated: number;
  widened_pool_n: number;
  original_pool_n: number;
  constraints_tested: string[];
  expected_spurious_collisions: number;
  inconclusive: boolean;
  rival?: Substitution;
  admitting_constraint?: string;
  note: string;
}

export interface Narrowing {
  pool_before: number;
  pool_after: number;
  dropped: Record<string, number>;
  window_hours: number;
  constraints_applied: string[];
  neighbourhood_probe?: Neighbourhood;
  completeness_checks: Check[];
  zero_contribution_records?: string[];
}

export interface Equation {
  gross_paise: number;
  mdr_paise: number;
  gst_on_mdr_paise: number;
  refunds_paise: number;
  chargebacks_paise: number;
  adjustments_paise: number;
  reconstructed_paise: number;
  target_paise: number;
  residual_paise: number;
  slack_allowed_paise: number;
  slack_consumed_paise: number;
  closes: boolean;
  negative_items: number;
}

export interface FeeCheck {
  mode: string;
  circular: boolean;
  expected_mdr_paise: number;
  observed_mdr_paise: number;
  delta_bps: number;
  band_bps: number;
  rounding_component_bps: number;
  within_band: boolean;
  claim: string;
}

export interface Hypothesis {
  kind: string;
  amount_paise: number;
  effect: string;
  source_ref?: string;
  evidence?: string;
  rationale?: string;
  outcome?: string;
}

export interface AgentBlock {
  invoked: boolean;
  provider?: string;
  iterations?: number;
  hypotheses?: Hypothesis[];
  accepted?: Hypothesis;
  note?: string;
}

export interface Receipt {
  settlement_ref: string;
  run_id: string;
  merchant_id: string;
  merchant_name?: string;
  merchant_archetype?: string;
  status: Status;
  flags: string[];
  data_mode: string;
  narration?: string;
  target_paise: number;
  value_date: string;
  pool: {
    n: number;
    contribution_sigma_paise: number;
    signed_items: number;
    total_contribution_paise: number;
  };
  amount_entropy: Entropy;
  feasibility: Feasibility;
  solver?: SolverBlock;
  uniqueness?: Uniqueness;
  witness?: string[];
  witness_size: number;
  negative_members?: string[];
  accounting?: Equation;
  rounding: {
    mode: string;
    tolerance_paise: number;
    band_basis: string;
    slack_allowed_paise: number;
    slack_consumed_paise: number;
  };
  narrowing: Narrowing;
  fee_check?: FeeCheck;
  agent: AgentBlock;
  claim: string;
  note?: string;
  remediation?: Remediation[];
  exception_cost_inr?: number;
  timing_ms: Record<string, number>;
  policy_version: string;
  replay_seed: number;
}

export interface ArchetypeResult {
  archetype: string;
  expected_regime: string;
  settlements: number;
  auto_post_rate: number;
  auto_posted_wrong: number;
  mean_sigma_paise: number;
  mean_twin_mass: number;
  mean_collision_index: number;
  entropy_gate_refusals: number;
  b0_post_rate: number;
  b0_wrong_post_rate: number;
}

export interface Summary {
  run_id: string;
  settlements: number;
  seed: number;
  status_counts: Record<string, number>;
  flag_counts: Record<string, number>;
  auto_posted: number;
  auto_posted_wrong: number;
  exceptions: number;
  b0_auto_posted: number;
  b0_auto_posted_wrong: number;
  b0_unresolved: number;
  median_latency_ms: number;
  p95_latency_ms: number;
  b0_median_latency_ms: number;
  wall_clock_s: number;
  settlements_per_hour: number;
  peak_memory_mb: number;
  parse_calls: number;
  agent_calls: number;
  model_calls: number;
  input_tokens: number;
  exception_rate: number;
  inr_per_1k_settlements: number;
  b0_input_tokens_per_1k: number;
  input_tokens_per_1k: number;
  b0_inr_per_1k_settlements: number;
  provider: string;
  provider_models: string;
  priced_at_model: string;
  price_is_real_spend: boolean;
  by_archetype: ArchetypeResult[];
  narrowing_drift?: {
    constraint: string;
    drop_rate_observed: number;
    drop_rate_baseline: number;
    baseline_source: string;
    gate: string;
    note: string;
  }[];
}

export interface CaseOutcome {
  case: {
    Number: number;
    Name: string;
    Scenario: string;
    ExpectB0: string;
    ExpectAxiom: string;
    Why: string;
  };
  status: Status;
  flags: string[];
  posted: boolean;
  posted_wrong: boolean;
  pool_n: number;
  k_star: number;
  collision_index: number;
  latency_ms: number;
  receipt: Receipt;
  b0_posted: boolean;
  b0_posted_wrong: boolean;
  b0_confidence: number;
  b0_proposed: string[] | null;
  b0_tokens_in: number;
  expectation_met: boolean;
  note?: string;
}

export interface SweepPoint {
  archetype: string;
  pool_n: number;
  batch_size: number;
  trials: number;
  mean_collision_index: number;
  mean_analytic_collision_index: number;
  mean_k_star: number;
  mean_sigma_paise: number;
  mean_twin_mass: number;
  verified_rate: number;
  ambiguous_rate: number;
  underdetermined_rate: number;
  narrowing_sensitive_rate: number;
  unresolved_rate: number;
  wrong_post_rate: number;
  b0_wrong_post_rate: number;
  b0_post_rate: number;
  mean_reconstructions_counted: number;
  entropy_gate_refusal_rate: number;
  mean_latency_ms: number;
}

export interface EnvelopePoint {
  pool_n: number;
  k_star: number;
  predicted_entries: number;
  observed_entries: number;
  predicted_mb: number;
  observed_mb: number;
  solve_and_prove_ms: number;
  uniqueness_included: boolean;
}

export interface Citation {
  receipt_id: string;
  field: string;
  value?: string;
}

export interface Answer {
  answer: string;
  citations: Citation[];
  answerable: boolean;
  retrieved: string[] | null;
}
