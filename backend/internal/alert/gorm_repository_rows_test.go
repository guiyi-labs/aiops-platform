package alert

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// 回归用例：`rows.Next()` 返回 false 有两种含义——"读完了"和"中途出错"。
// 三者（ListRules / ListInstances / ClaimDueRules）在修复前都不检查 `rows.Err()`，
// 于是把后者当成前者，以 nil 错误返回**部分结果**：
// 调用方无法区分「匹配 3 条」与「匹配 300 条但读到第 3 条断了」。
//
// 这类缺陷的特征是不报错、不 panic、无日志——常规信号全部指向"正常"，
// 只能靠"注入一个迭代中途的失败，断言它必须冒出来"来锁住。

var errInjectedMidIteration = errors.New("injected mid-iteration failure")

// --- sqlmock scaffolding -------------------------------------------------

// sqlFragmentMatcher 把期望 SQL 当成"空白归一化后的子串"来匹配。
// sqlmock 默认匹配器把期望编译成正则，逼着调用方转义 Postgres SQL 里
// 每一个括号、引号和美元符号；改用特征片段可以钉住语句形状（表名、ORDER BY、LIMIT）
// 而不必硬编码占位符编号与完整列清单。
func sqlFragmentMatcher(expected, actual string) error {
	if strings.Contains(collapseSQL(actual), collapseSQL(expected)) {
		return nil
	}
	return fmt.Errorf("actual sql %q does not contain fragment %q", collapseSQL(actual), collapseSQL(expected))
}

func collapseSQL(q string) string { return strings.Join(strings.Fields(q), " ") }

func newAlertMockGorm(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherFunc(sqlFragmentMatcher)))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	gdb, err := gorm.Open(gormpostgres.New(gormpostgres.Config{Conn: db}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open with sqlmock: %v", err)
	}
	return gdb, mock
}

// --- fixtures ------------------------------------------------------------

// 列顺序必须与 ListRules 的 SELECT 和 scanRule 的 Scan 参数顺序一致。
var ruleColumns = []string{
	"id", "cluster_id", "display_name", "resource_kind", "resource_name", "metric_name", "operator",
	"threshold", "for_seconds", "minimum_points", "enabled", "deleted", "last_evaluation_state",
	"last_evaluation_at", "last_error_code", "next_due_at", "claim_expires_at",
	"creator_user_id", "creator_name", "created_at", "updated_at",
}

func ruleRowValues(now time.Time) []driver.Value {
	return []driver.Value{
		int64(1), int64(2), "rule-a", "Deployment", "api", "cpu", ">",
		int64(80), int64(60), int64(3), true, false, "ok",
		nil, "", now, nil,
		int64(7), "alice", now, now,
	}
}

// ruleRowValues2 是同一结果集的第 2 行。注入迭代中途失败时需要它——见
// TestGormRepositoryListRulesSurfacesMidIterationError 里的说明。
func ruleRowValues2(now time.Time) []driver.Value {
	return []driver.Value{
		int64(2), int64(2), "rule-b", "StatefulSet", "db", "memory", ">",
		int64(90), int64(120), int64(5), true, false, "ok",
		nil, "", now, nil,
		int64(7), "alice", now, now,
	}
}

// 列顺序必须与 ListInstances 的 SELECT 和 scanInstance 的 Scan 参数顺序一致。
var instanceColumns = []string{
	"id", "rule_id", "diagnosis_id", "state", "first_fired_at", "last_fired_at",
	"resolved_at", "latest_evidence_anchor", "created_at", "updated_at",
}

func instanceRowValues(now time.Time) []driver.Value {
	return []driver.Value{
		int64(1), int64(2), int64(3), "firing", now, now,
		nil, `{}`, now, now,
	}
}

// instanceRowValues2 见 ruleRowValues2 的说明。
func instanceRowValues2(now time.Time) []driver.Value {
	return []driver.Value{
		int64(2), int64(2), int64(4), "resolved", now, now,
		now, `{}`, now, now,
	}
}

// --- ListRules -----------------------------------------------------------

// 阳性对照：先证明 fixture 本身是有效的。
// 没有这一条，下面的"必须报错"用例可能因为 fixture 根本扫不过去而**空过**。
func TestGormRepositoryListRulesReturnsCompleteResultOnSuccess(t *testing.T) {
	gdb, mock := newAlertMockGorm(t)
	now := time.Now().UTC()
	mock.ExpectQuery("FROM alert_rules").
		WillReturnRows(sqlmock.NewRows(ruleColumns).AddRow(ruleRowValues(now)...))

	rules, err := NewGormRepository(gdb).ListRules(context.Background(), RuleListFilter{ClusterID: 2, Limit: 10})
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("rules = %d, want 1", len(rules))
	}
	if rules[0].ID != 1 || rules[0].DisplayName != "rule-a" {
		t.Fatalf("unexpected rule: %+v", rules[0])
	}
}

func TestGormRepositoryListRulesSurfacesMidIterationError(t *testing.T) {
	gdb, mock := newAlertMockGorm(t)
	now := time.Now().UTC()
	// 2 行 + 在第 2 次读取（交付第 1 行时）注入失败。
	//
	// 为什么必须给 2 行：sqlmock 的 driver.Rows.Next 只在**交付某一行时**才返回
	// nextErr[pos-1]，行用完后一律返回 io.EOF 而不再查 nextErr。所以
	// 「1 行 + RowError(1, ...)」永远不触发——必须多给一行，错误才有地方落。
	//
	// 这样循环体正好执行一次：收集到 1 条，随后失败。
	mock.ExpectQuery("FROM alert_rules").
		WillReturnRows(sqlmock.NewRows(ruleColumns).
			AddRow(ruleRowValues(now)...).
			AddRow(ruleRowValues2(now)...).
			RowError(1, errInjectedMidIteration))

	rules, err := NewGormRepository(gdb).ListRules(context.Background(), RuleListFilter{ClusterID: 2, Limit: 10})

	// 修复前：err == nil 且 rules 只含 1 条 —— 部分结果被当成完整结果返回。
	if !errors.Is(err, errInjectedMidIteration) {
		t.Fatalf("err = %v, want %v（部分结果不能以 nil 错误返回）", err, errInjectedMidIteration)
	}
	if rules != nil {
		t.Fatalf("rules = %+v, want nil（出错时不应返回部分结果）", rules)
	}
}

// --- ListInstances -------------------------------------------------------

func TestGormRepositoryListInstancesReturnsCompleteResultOnSuccess(t *testing.T) {
	gdb, mock := newAlertMockGorm(t)
	now := time.Now().UTC()
	mock.ExpectQuery("FROM alert_instances").
		WillReturnRows(sqlmock.NewRows(instanceColumns).AddRow(instanceRowValues(now)...))

	instances, err := NewGormRepository(gdb).ListInstances(context.Background(), InstanceListFilter{ClusterID: 2, Limit: 10})
	if err != nil {
		t.Fatalf("ListInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("instances = %d, want 1", len(instances))
	}
	if instances[0].ID != 1 || instances[0].State != "firing" {
		t.Fatalf("unexpected instance: %+v", instances[0])
	}
}

func TestGormRepositoryListInstancesSurfacesMidIterationError(t *testing.T) {
	gdb, mock := newAlertMockGorm(t)
	now := time.Now().UTC()
	mock.ExpectQuery("FROM alert_instances").
		WillReturnRows(sqlmock.NewRows(instanceColumns).
			AddRow(instanceRowValues(now)...).
			AddRow(instanceRowValues2(now)...).
			RowError(1, errInjectedMidIteration))

	instances, err := NewGormRepository(gdb).ListInstances(context.Background(), InstanceListFilter{ClusterID: 2, Limit: 10})

	if !errors.Is(err, errInjectedMidIteration) {
		t.Fatalf("err = %v, want %v（部分结果不能以 nil 错误返回）", err, errInjectedMidIteration)
	}
	if instances != nil {
		t.Fatalf("instances = %+v, want nil（出错时不应返回部分结果）", instances)
	}
}

// --- ClaimDueRules -------------------------------------------------------

func TestGormRepositoryClaimDueRulesSurfacesMidIterationError(t *testing.T) {
	gdb, mock := newAlertMockGorm(t)
	now := time.Now().UTC()

	// 第一步：UPDATE ... RETURNING id 把待评规则 claim 掉。
	mock.ExpectExec("UPDATE alert_rules SET claim_expires_at").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// 第二步：重查已 claim 的行 —— 这里注入迭代中途的失败。
	mock.ExpectQuery("FROM alert_rules").
		WillReturnRows(sqlmock.NewRows(ruleColumns).
			AddRow(ruleRowValues(now)...).
			AddRow(ruleRowValues2(now)...).
			RowError(1, errInjectedMidIteration))

	rules, err := NewGormRepository(gdb).ClaimDueRules(context.Background(), now, 10, 2*time.Minute)

	// 这一步尤其要紧：行已经在 UPDATE 里被 claim 掉了。若静默返回部分结果，
	// 未返回的那部分会一直被 claim 到租约过期才被重新拾起——表现为"漏评一轮且无任何错误信号"。
	if !errors.Is(err, errInjectedMidIteration) {
		t.Fatalf("err = %v, want %v", err, errInjectedMidIteration)
	}
	if rules != nil {
		t.Fatalf("rules = %+v, want nil", rules)
	}
}
