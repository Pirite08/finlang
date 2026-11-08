package eval

import (
	"testing"

	"github.com/Pirite08/finlang/internal/config"
	"github.com/Pirite08/finlang/internal/diagnostic"
	"github.com/Pirite08/finlang/internal/trace"
)

func TestCollectionRuleRunsApprovedOperations(t *testing.T) {
	rule := parseRuleForTest(t, `rule checked {
		input amount in "USD"
		output result in "USD"
		step calculate {
			let products = [record { amount: amount }, record { amount: amount }]
			var totals = map {}
			for product in enumerate(products) {
				let code = currency(product.value.amount)
				let previous = if (totals.has(code)) { totals[code] } else { money(code, 0) }
				totals = totals.with(code, previous + product.value.amount)
			}
			require all([any([true]), len(range(0, 2)) == 2, totals.len() == 1])
			require lower_bound(sorted([2, 1]), 2) == 1
			require sum([1, zip_exact([2], [3])[0].left]) == 3
			let result = totals.get(totals.keys()[0])
		}
		return result
	}`)
	result, err := New(Options{RuleOnly: true, CollectionRules: true}).RunRule(rule, ruleMoney(t, "USD", "1.25"))
	if err != nil || result.Inspect() != "USD 2.5" {
		t.Fatalf("approved collection operations: result=%v err=%v", result, err)
	}
}

func TestCollectionRuleRejectsUnapprovedOperationsBeforeSteps(t *testing.T) {
	for _, statement := range []string{
		`let ignored = queue()`,
		`let ignored = matrix([[1]])`,
		`let ignored = map {}.dequeue()`,
		`let ignored = amount.currency()`,
		`let ignored = print(amount)`,
		`let ignored = [amount][0]()`,
		`let ignored = fn() { return amount }`,
		`let ignored = enumerate`,
		`var range = amount`,
	} {
		t.Run(statement, func(t *testing.T) {
			rule := parseRuleForTest(t, `rule checked {
				input amount in "USD"
				output result in "USD"
				step calculate { let result = amount }
				step checked { if (false) { `+statement+` } }
				return result
			}`)
			recorder := trace.NewRecorder(50)
			_, err := New(Options{RuleOnly: true, CollectionRules: true, Trace: recorder}).RunRule(rule, ruleMoney(t, "USD", "1"))
			if diagnostic.CodeOf(err) != "E_RULE_POLICY" || len(recorder.Events()) != 0 {
				t.Fatalf("unsafe operation: err=%v events=%+v", err, recorder.Events())
			}
		})
	}
}

func TestCollectionRuleRejectsUnapprovedMethodReceiver(t *testing.T) {
	rule := parseRuleForTest(t, `rule checked {
		input amount in "USD"
		output result in "USD"
		step calculate { let ignored = [amount].len(); let result = amount }
		return result
	}`)
	_, err := New(Options{RuleOnly: true, CollectionRules: true}).RunRule(rule, ruleMoney(t, "USD", "1"))
	if diagnostic.CodeOf(err) != "E_RULE_POLICY" {
		t.Fatalf("non-Map receiver should be rejected: %v", err)
	}
}

func TestCollectionRuleLoopUsesExecutionBudget(t *testing.T) {
	rule := parseRuleForTest(t, `rule checked {
		input amount in "USD"
		output result in "USD"
		step calculate { for row in range(0, 50) {}; let result = amount }
		return result
	}`)
	limits := config.DefaultLimits()
	limits.MaxEvalSteps = 30
	_, err := New(Options{RuleOnly: true, CollectionRules: true, Limits: limits}).RunRule(rule, ruleMoney(t, "USD", "1"))
	if diagnostic.CodeOf(err) != "E_EXECUTION_LIMIT" {
		t.Fatalf("loop should exceed the execution limit: %v", err)
	}
}

func TestCollectionRuleEnforcesPerCollectionLimit(t *testing.T) {
	for _, tc := range []struct {
		name, statements string
		maxItems         int
	}{
		{"list literal", `let oversized = [0, 1, 2]`, 2},
		{"record literal", `let oversized = record { a: 0, b: 1, c: 2 }`, 2},
		{"map literal", `let oversized = map { "a": 0, "b": 1, "c": 2 }`, 2},
		{"dead branch literal", `if (false) { let oversized = [0, 1, 2] }`, 2},
		{"lazy range", `let oversized = range(0, 3)`, 2},
		{"map growth", `let oversized = map { "a": 0, "b": 1 }.with("c", 2)`, 2},
		{"derived record width", `let oversized = enumerate([0])`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rule := parseRuleForTest(t, `rule checked {
				input amount in "USD"
				output result in "USD"
				step calculate { `+tc.statements+`; let result = amount }
				return result
			}`)
			limits := config.DefaultLimits()
			limits.MaxCollectionItems = tc.maxItems
			_, err := New(Options{RuleOnly: true, CollectionRules: true, Limits: limits}).RunRule(rule, ruleMoney(t, "USD", "1"))
			if diagnostic.CodeOf(err) != "E_COLLECTION_LIMIT" {
				t.Fatalf("expected collection size error, got %v", err)
			}
		})
	}
}

func TestCollectionRuleTakesListOfRecordsAndReturnsMap(t *testing.T) {
	rule := parseRuleForTest(t, `rule checked {
		input rows as list
		output totals as map
		step calculate { let totals = map { "USD": rows[0].value }.with("USD", rows[0].value) }
		return totals
	}`)
	amount := ruleMoney(t, "USD", "2.00")
	fields := []RecordField{{Name: "value", Value: amount}}
	items := []Value{NewRecordValue(fields)}
	input := NewListValue(items)
	fields[0].Value = StringValue("changed")
	items[0] = StringValue("changed")
	limits := config.DefaultLimits()
	limits.MaxCollectionItems = 1
	e := New(Options{RuleOnly: true, CollectionRules: true, Limits: limits})
	result, err := e.RunRule(rule, input)
	entries, ok := MapEntries(result)
	if err != nil || !ok || len(entries) != 1 || entries[0].Key != "USD" || entries[0].Value.Inspect() != "USD 2" {
		t.Fatalf("list of records to map: entries=%v result=%v err=%v", entries, result, err)
	}
	entries[0].Value = StringValue("changed")
	unchanged, _ := MapEntries(result)
	if unchanged[0].Value.Inspect() != "USD 2" {
		t.Fatalf("caller changed the returned Map: %v", unchanged)
	}
	for _, tc := range []struct {
		name  string
		input Value
		code  diagnostic.Code
	}{
		{"wrong input kind", amount, "E_RULE_INPUT"},
		{"non-record item", NewListValue([]Value{amount}), "E_RULE_INPUT"},
		{"too many rows", NewListValue([]Value{items[0], items[0]}), "E_COLLECTION_LIMIT"},
		{"too many fields", NewListValue([]Value{NewRecordValue([]RecordField{{"a", amount}, {"b", amount}})}), "E_COLLECTION_LIMIT"},
		{"disallowed field value", NewListValue([]Value{NewRecordValue([]RecordField{{"value", input}})}), "E_RULE_INPUT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.RunRule(rule, tc.input)
			if diagnostic.CodeOf(err) != tc.code {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}
	if _, err := New(Options{RuleOnly: true}).RunRule(rule, input); diagnostic.CodeOf(err) != "E_RULE_POLICY" {
		t.Fatalf("legacy profile should reject collection declarations: %v", err)
	}
}

func TestCollectionRuleTakesAndReturnsRecord(t *testing.T) {
	rule := parseRuleForTest(t, `rule transfer_quote {
		input transfer as record
		output quote as record
		step calculate {
			let fee = settle(transfer.amount * 0.05, 0, "HALF_EVEN")
			let quote = record { fee: fee, total_debit: transfer.amount + fee }
		}
		return quote
	}`)
	amount := ruleMoney(t, "KRW", "10000")
	input := NewRecordValue([]RecordField{
		{Name: "request_id", Value: StringValue("quote-1")},
		{Name: "amount", Value: amount},
	})
	limits := config.DefaultLimits()
	limits.MaxCollectionItems = 2
	e := New(Options{RuleOnly: true, CollectionRules: true, Limits: limits})
	result, err := e.RunRule(rule, input)
	fields, ok := RecordFields(result)
	if err != nil || !ok || len(fields) != 2 || fields[0].Name != "fee" || fields[0].Value.Inspect() != "KRW 500" ||
		fields[1].Name != "total_debit" || fields[1].Value.Inspect() != "KRW 10500" {
		t.Fatalf("record input and output: result=%v err=%v", result, err)
	}
	fields[0].Value = StringValue("changed")
	unchanged, _ := RecordFields(result)
	if unchanged[0].Value.Inspect() != "KRW 500" {
		t.Fatalf("caller changed the returned Record: %v", unchanged)
	}
	for _, tc := range []struct {
		name  string
		input Value
		code  diagnostic.Code
	}{
		{"wrong input kind", amount, "E_RULE_INPUT"},
		{"too many fields", NewRecordValue([]RecordField{{"a", amount}, {"b", amount}, {"c", amount}}), "E_COLLECTION_LIMIT"},
		{"disallowed field kind", NewRecordValue([]RecordField{{"amount", input}}), "E_RULE_INPUT"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.RunRule(rule, tc.input)
			if diagnostic.CodeOf(err) != tc.code {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}
	for _, options := range []Options{{RuleOnly: true}, {}} {
		if _, err := New(options).RunRule(rule, input); diagnostic.CodeOf(err) != "E_RULE_POLICY" {
			t.Fatalf("non-collection profile should reject Record declarations: %v", err)
		}
	}
}

func TestCollectionRuleRequiresDeclaredRecordOutput(t *testing.T) {
	rule := parseRuleForTest(t, `rule checked {
		input transfer as record
		output quote as record
		step calculate { let quote = transfer.amount }
		return quote
	}`)
	input := NewRecordValue([]RecordField{{Name: "amount", Value: ruleMoney(t, "KRW", "10000")}})
	if _, err := New(Options{RuleOnly: true, CollectionRules: true}).RunRule(rule, input); diagnostic.CodeOf(err) != "E_RULE_OUTPUT" {
		t.Fatalf("expected E_RULE_OUTPUT, got %v", err)
	}
}
