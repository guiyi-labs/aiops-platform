package slo

import (
	"testing"
)

// TestValidateDefinition_RequiredFields verifies the validation invariants
// for the Definition model.
func TestValidateDefinition_RequiredFields(t *testing.T) {
	tests := []struct {
		name    string
		mut     func(*Definition)
		wantErr bool
	}{
		{"valid", func(d *Definition) {}, false},
		{"nil_definition", func(d *Definition) { *d = Definition{} }, true},
		{"missing_cluster_id", func(d *Definition) { d.ClusterID = 0 }, true},
		{"missing_service_kind", func(d *Definition) { d.Service.Kind = "" }, true},
		{"missing_service_name", func(d *Definition) { d.Service.Name = "" }, true},
		{"service_name_too_long", func(d *Definition) { d.Service.Name = string(make([]byte, MaxServiceNameLength+1)) }, true},
		{"namespace_too_long", func(d *Definition) { d.Service.Namespace = string(make([]byte, MaxNamespaceLength+1)) }, true},
		{"bad_template", func(d *Definition) { d.Template = SLITemplate("nope") }, true},
		{"objective_below_min", func(d *Definition) { d.Objective = -0.1 }, true},
		{"objective_above_max", func(d *Definition) { d.Objective = 1.1 }, true},
		{"rolling_window_too_short", func(d *Definition) { d.RollingWindowSeconds = 60 }, true},
		{"rolling_window_too_long", func(d *Definition) { d.RollingWindowSeconds = MaxRollingWindowSeconds + 1 }, true},
		{"bad_missing_data_policy", func(d *Definition) { d.MissingDataPolicy = MissingDataPolicy("nope") }, true},
		{"fail_open_on_request_template", func(d *Definition) {
			d.Template = TemplateRequestSuccessRatio
			d.MissingDataPolicy = MissingDataFailOpen
		}, true},
		{"fail_open_on_workload_readiness_ok", func(d *Definition) {
			d.Template = TemplateWorkloadReadiness
			d.MissingDataPolicy = MissingDataFailOpen
		}, false},
		{"latency_ms_on_non_latency_template", func(d *Definition) {
			d.Template = TemplateRequestSuccessRatio
			d.LatencyThresholdMs = 100
		}, true},
		{"latency_ms_required_for_latency_template", func(d *Definition) {
			d.Template = TemplateRequestLatencyTargetRatio
			d.LatencyThresholdMs = 0
		}, true},
		{"latency_ms_in_range_for_latency_template", func(d *Definition) {
			d.Template = TemplateRequestLatencyTargetRatio
			d.LatencyThresholdMs = 500
		}, false},
		{"latency_ms_below_min", func(d *Definition) {
			d.Template = TemplateRequestLatencyTargetRatio
			d.LatencyThresholdMs = 0
		}, true},
		{"latency_ms_above_max", func(d *Definition) {
			d.Template = TemplateRequestLatencyTargetRatio
			d.LatencyThresholdMs = MaxLatencyThresholdMs + 1
		}, true},
		{"fast_burn_rate_negative", func(d *Definition) { d.FastBurnRate = -1 }, true},
		{"slow_burn_rate_negative", func(d *Definition) { d.SlowBurnRate = -1 }, true},
		{"fast_burn_window_too_short", func(d *Definition) { d.FastBurnWindowSeconds = 30 }, true},
		{"fast_burn_window_too_long", func(d *Definition) { d.FastBurnWindowSeconds = MaxBurnWindowSeconds + 1 }, true},
		{"slow_burn_window_too_short", func(d *Definition) { d.SlowBurnWindowSeconds = 30 }, true},
		{"fast_burn_window_gt_slow_burn_window", func(d *Definition) {
			d.FastBurnWindowSeconds = 7200
			d.SlowBurnWindowSeconds = 3600
		}, true},
		{"missing_template_version", func(d *Definition) { d.TemplateVersion = "" }, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			def := &Definition{
				ClusterID:             1,
				Service:               ServiceRef{Kind: "Deployment", Namespace: "default", Name: "api"},
				Template:              TemplateRequestSuccessRatio,
				TemplateVersion:       TemplateVersion,
				Objective:             0.99,
				RollingWindowSeconds:  3600,
				MissingDataPolicy:     MissingDataUnavailable,
				FastBurnRate:          14.4,
				FastBurnWindowSeconds: 3600,
				SlowBurnRate:          1.0,
				SlowBurnWindowSeconds: 21600,
			}
			tc.mut(def)
			err := ValidateDefinition(def)
			if tc.wantErr && err == nil {
				t.Errorf("expected error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

// TestValidateCreate_RequiresCreatorAndOwner verifies creator/owner IDs.
func TestValidateCreate_RequiresCreatorAndOwner(t *testing.T) {
	in := validCreateInput()
	if err := ValidateCreate(in); err != nil {
		t.Fatalf("valid input should pass, got %v", err)
	}
	in.Creator = ActorRef{}
	if err := ValidateCreate(in); err == nil {
		t.Errorf("missing creator should fail")
	}
	in = validCreateInput()
	in.Owner = ActorRef{}
	if err := ValidateCreate(in); err == nil {
		t.Errorf("missing owner should fail")
	}
}

// TestLookupTemplate verifies the catalog lookup.
func TestLookupTemplate(t *testing.T) {
	for _, tmpl := range []SLITemplate{TemplateRequestSuccessRatio, TemplateRequestLatencyTargetRatio, TemplateWorkloadReadiness} {
		desc, ok := LookupTemplate(tmpl)
		if !ok {
			t.Errorf("template %s should be in catalog", tmpl)
		}
		if desc.Template != tmpl {
			t.Errorf("descriptor template mismatch: want %s, got %s", tmpl, desc.Template)
		}
	}
	_, ok := LookupTemplate(SLITemplate("nope"))
	if ok {
		t.Errorf("unknown template should not be in catalog")
	}
}

// TestAllTemplates verifies the catalog returns all 3 templates.
func TestAllTemplates(t *testing.T) {
	all := AllTemplates()
	if len(all) != 3 {
		t.Errorf("want 3 templates, got %d", len(all))
	}
}

// TestDefaultMissingDataPolicy verifies the default policy is "unavailable"
// for every template, even those that allow fail_open.
func TestDefaultMissingDataPolicy(t *testing.T) {
	for _, tmpl := range []SLITemplate{TemplateRequestSuccessRatio, TemplateRequestLatencyTargetRatio, TemplateWorkloadReadiness} {
		if got := DefaultMissingDataPolicy(tmpl); got != MissingDataUnavailable {
			t.Errorf("default for %s: want unavailable, got %s", tmpl, got)
		}
	}
	if got := DefaultMissingDataPolicy(SLITemplate("nope")); got != MissingDataUnavailable {
		t.Errorf("default for unknown: want unavailable, got %s", got)
	}
}
