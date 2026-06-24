// Copyright 2025 The OpenChoreo Authors
// SPDX-License-Identifier: Apache-2.0

package casbin

import (
	"testing"

	authzv1alpha1 "github.com/openchoreo/openchoreo/api/v1alpha1"
	authzcore "github.com/openchoreo/openchoreo/internal/authz/core"
)

// benchExprs are representative authz CEL conditions (only `resource` is declared in the CEL env).
var benchExprs = []string{
	`resource.environment == "production"`,
	`resource.environment == "dev" || resource.environment == "staging"`,
	`resource.project == "payments" && resource.environment != "production"`,
	`resource.environment == "production" && resource.project != "sandbox"`,
}

// BenchmarkConditionMatcher measures the full per-request, per-conditional-policy authz cost:
// unmarshal condition JSON + filter by action + build CEL activation + compile + eval. This is the
// representative number — list endpoints run it once per item. Run with and without the patch to
// see the delta (compilation is the part the cache removes).
func BenchmarkConditionMatcher(b *testing.B) {
	ctx, _ := serializeAuthzContext(authzcore.Context{Resource: authzcore.ResourceAttribute{Environment: "dev"}})
	cond, _ := serializeAuthzConditions([]authzv1alpha1.AuthzCondition{
		{Actions: []string{"releasebinding:create"}, Expression: `resource.environment == "dev"`},
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !ConditionMatcher(ctx, "releasebinding:create", cond, "allow", "bench") {
			b.Fatal("expected match")
		}
	}
}

// BenchmarkCompileCEL measures the cost of compileCEL, which evalCondition invokes on every
// condition evaluation (per conditional policy, and per item on list endpoints).
func BenchmarkCompileCEL(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := compileCEL(benchExprs[i%len(benchExprs)]); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompileAndEval mirrors the real hot path: obtain the program, then evaluate it.
func BenchmarkCompileAndEval(b *testing.B) {
	act := map[string]interface{}{
		"resource": map[string]interface{}{"environment": "production", "project": "payments"},
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		prg, err := compileCEL(benchExprs[i%len(benchExprs)])
		if err != nil {
			b.Fatal(err)
		}
		_, _, _ = prg.Eval(act)
	}
}
