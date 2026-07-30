package alert

import (
	"testing"
)

func TestValidateCreate(t *testing.T) {
	validInput := CreateRuleInput{
		ClusterID:     1,
		DisplayName:   "High CPU Alert",
		ResourceKind:  ResourceKindNode,
		ResourceName:  "worker-01",
		MetricName:    MetricCPU,
		Operator:      OperatorGTE,
		Threshold:     3000000000,
		ForSeconds:    300,
		MinimumPoints: 5,
	}

	t.Run("valid input passes", func(t *testing.T) {
		if err := ValidateCreate(validInput); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("zero cluster ID fails", func(t *testing.T) {
		input := validInput
		input.ClusterID = 0
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("empty display name fails", func(t *testing.T) {
		input := validInput
		input.DisplayName = ""
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("display name too long fails", func(t *testing.T) {
		input := validInput
		input.DisplayName = string(make([]byte, 129))
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("invalid resource kind fails", func(t *testing.T) {
		input := validInput
		input.ResourceKind = "Pod"
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("empty resource name fails", func(t *testing.T) {
		input := validInput
		input.ResourceName = ""
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("invalid metric name fails", func(t *testing.T) {
		input := validInput
		input.MetricName = "disk"
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("invalid operator fails", func(t *testing.T) {
		input := validInput
		input.Operator = "gt"
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("negative threshold fails", func(t *testing.T) {
		input := validInput
		input.Threshold = -1
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("for_seconds below minimum fails", func(t *testing.T) {
		input := validInput
		input.ForSeconds = 59
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("for_seconds above maximum fails", func(t *testing.T) {
		input := validInput
		input.ForSeconds = 21601
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("minimum_points below minimum fails", func(t *testing.T) {
		input := validInput
		input.MinimumPoints = 1
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("minimum_points above maximum fails", func(t *testing.T) {
		input := validInput
		input.MinimumPoints = 361
		if err := ValidateCreate(input); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("zero threshold is valid", func(t *testing.T) {
		input := validInput
		input.Threshold = 0
		input.Operator = OperatorLTE
		if err := ValidateCreate(input); err != nil {
			t.Errorf("expected no error for zero threshold with lte, got %v", err)
		}
	})

	t.Run("memory metric with lte is valid", func(t *testing.T) {
		input := validInput
		input.MetricName = MetricMemory
		input.Operator = OperatorLTE
		input.Threshold = 1000000
		if err := ValidateCreate(input); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestValidatePatch(t *testing.T) {
	t.Run("valid display name passes", func(t *testing.T) {
		name := "Updated Alert"
		if err := ValidatePatch(PatchRuleInput{DisplayName: &name}); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("empty display name fails", func(t *testing.T) {
		name := ""
		if err := ValidatePatch(PatchRuleInput{DisplayName: &name}); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("display name too long fails", func(t *testing.T) {
		name := string(make([]byte, 129))
		if err := ValidatePatch(PatchRuleInput{DisplayName: &name}); err != ErrInvalidRule {
			t.Errorf("expected ErrInvalidRule, got %v", err)
		}
	})

	t.Run("enabled only passes", func(t *testing.T) {
		enabled := false
		if err := ValidatePatch(PatchRuleInput{Enabled: &enabled}); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("nil fields passes", func(t *testing.T) {
		if err := ValidatePatch(PatchRuleInput{}); err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})
}

func TestConstants(t *testing.T) {
	if MaxRulesPerCluster != 20 {
		t.Errorf("MaxRulesPerCluster = %d, want 20", MaxRulesPerCluster)
	}
	if MinForSeconds != 60 {
		t.Errorf("MinForSeconds = %d, want 60", MinForSeconds)
	}
	if MaxForSeconds != 21600 {
		t.Errorf("MaxForSeconds = %d, want 21600", MaxForSeconds)
	}
	if MinMinimumPoints != 2 {
		t.Errorf("MinMinimumPoints = %d, want 2", MinMinimumPoints)
	}
	if MaxMinimumPoints != 360 {
		t.Errorf("MaxMinimumPoints = %d, want 360", MaxMinimumPoints)
	}
}
