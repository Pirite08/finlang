# 금융 계산 인터프리터 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (- [ ]) syntax for tracking.

**Goal:** 소수 계산·통화·명시적 정산·계산 근거를 갖춘 Go 인터프리터를 만들고, 교육용 COBOL의 작은 계산 규칙 3개와 비교한다.

**Architecture:** big.Int 계수와 스케일을 갖는 Decimal을 독립 모듈로 구현한다. lexer, Pratt parser, AST 평가기로 언어를 실행하고 Money·JSON 계약·trace를 연결한다. COBOL 비교는 별도 테스트 실행기로 수행한다.

**Tech Stack:** Go 1.25 이상, Go 표준 라이브러리, Windows 개발 환경, 비교 단계의 Linux/WSL용 Go와 GnuCOBOL.

**Spec:** [금융 계산 인터프리터 최종 설계안](../specs/2026-10-31-financial-interpreter-design.md)

## Global Constraints

- 정규화된 계수의 최대 십진 자릿수는 128이다.
- Decimal 최대 스케일은 36이다.
- 반올림·나눗셈·format·format_money의 목표 스케일은 0~36이다.
- 숫자·식별자·문자열 토큰의 최대 크기는 256바이트다.
- 소스 파일 최대 크기는 1 MiB다.
- 구문 중첩 깊이는 128이다.
- 평가할 AST 노드 수는 실행당 100,000이다.
- 함수 호출 깊이는 64다.
- trace 이벤트 수는 10,000이며, 초과 시 잘림을 표시하고 수치 계산은 계속한다.
- 숫자는 원문에서 직접 해석하고 float64를 거치지 않는다.
- +, -, *는 허용 범위에서 정확하게 수행한다. 제한 초과는 오류다.
- div와 round에는 자리와 모드를 명시한다. 모드는 HALF_EVEN, HALF_UP, TOWARD_ZERO, EXACT다.
- Money의 초기 통화는 KRW, USD다. 자동 환전과 암묵적 숫자·통화 변환은 없다.
- 런타임 의존성은 Go 표준 라이브러리로 시작한다.
- 반복문·배열·맵·클래스·DB·네트워크·실제 송금·자동 COBOL 변환기는 범위 밖이다.
- COBOL 비교는 교육용 규칙과 고정된 GnuCOBOL 환경의 결과로 표현한다.

## Review Focus

1. big.Int 복사와 이름 재사용 때문에 원래 값이 변하지 않아야 한다. Task 2, 3, 7, 8의 불변성 테스트가 소유한다.
2. 음수의 정확한 절반, 연속 반올림, 정산 순서 차이를 구분해야 한다. Task 4, 10, 13이 소유한다.
3. 통화·타입 혼합과 JSON 숫자·중복 키 때문에 의미나 정밀도가 바뀌지 않아야 한다. Task 10, 11이 소유한다.
4. 자릿수 경계, 긴 구문, 재귀가 정상 진단으로 끝나야 한다. Task 2~9가 소유한다.
5. trace를 켜거나 상한을 넘겨도 수치 결과가 같고, COBOL 수신 필드 초과도 비교에 포함돼야 한다. Task 12, 13이 소유한다.

## 파일 배치

아래는 구현 때 만들 파일이다. 이 계획을 작성한 시점에는 설계 문서와 이 작업표만 존재한다.

| 위치 | 책임 |
|---|---|
| go.mod | 로컬 모듈 example.com/finlang, go 1.25 |
| cmd/fin/main.go | CLI 진입점 |
| internal/config/limits.go | 공통 제한값 |
| internal/diagnostic/diagnostic.go | 오류 코드와 소스 위치 |
| internal/decimal/*.go | 숫자 입력·정규화·연산·반올림·포맷 |
| internal/token/token.go | 토큰과 종류 |
| internal/lexer/lexer.go | 문자열·숫자 원문과 위치를 보존하는 렉서 |
| internal/ast/ast.go | 표현식과 문장 노드 |
| internal/parser/parser.go | 우선순위와 중첩을 다루는 파서 |
| internal/eval/*.go | 값·환경·평가기·내장 함수 |
| internal/cli/cli.go, internal/repl/repl.go | 파일·REPL·표준 입출력 |
| internal/money/money.go | 통화 의미와 정산 |
| internal/contract/*.go | JSON 계약·검증·직렬화 |
| internal/trace/*.go | 이벤트 구조와 제한된 기록기 |
| examples/*.fin | 단리·수수료·합산 정산 |
| contracts/*.json | 비교 규칙의 입력·출력 계약 |
| testdata/cobol/*.cob | 교육용 COBOL 기준 프로그램 |
| tests/compat/*.go | 외부 프로세스를 이용한 차등 비교 |
| experiments/*.go | 구현 방식 비교용 시험 코드 |
| docs/evaluation/*.md, *.jsonl, *.csv | 환경·원자료·결과 |

## 작업 공통 진행 방식

각 Task에서 아래 순서를 지킨다. 검증 명령은 구현 후 실행할 명령이며, 현재 실행 결과를 나타내지 않는다.

1. 명시한 입력·결과·오류를 테스트에 적는다.
2. 지정한 테스트를 실행해 새 기능이 아직 없다는 이유로 실패하는지 확인한다.
3. 정한 인터페이스로 필요한 구현을 추가한다.
4. 해당 테스트와 영향을 받는 기존 테스트를 실행한다.
5. 설명이 필요한 설계 변경을 기록하고, Git을 사용한다면 해당 작업 파일만 로컬 커밋한다.

배포와 원격 저장소 변경은 이 계획의 실행 범위에 포함하지 않는다.

## Task 1: 공통 계약과 실행 환경

**Files:** go.mod, internal/config/limits.go, internal/config/limits_test.go, internal/diagnostic/diagnostic.go, internal/diagnostic/diagnostic_test.go, internal/trace/event.go, experiments/representation_probe_test.go.

**Consumes:** 설계서의 제한·오류·trace 요구.

**Produces:**

    config.Limits
    config.DefaultLimits() Limits
    diagnostic.Code string
    diagnostic.Span { File string; Start, End, Line, Column int }
    diagnostic.Error { Code Code; Message string; Span *Span }
    diagnostic.New(code Code, message string) *Error
    diagnostic.WithSpan(err error, span Span) error
    diagnostic.CodeOf(err error) Code
    trace.Event
    trace.Sink interface { Record(Event) }

Limits의 필드는 MaxCoefficientDigits, MaxScale, MaxTokenBytes, MaxSourceBytes, MaxSyntaxDepth, MaxEvalSteps, MaxCallDepth, MaxTraceEvents다. 위치의 줄·열은 1부터 시작하고 열은 UTF-8 rune 기준이다. Start와 End는 바이트 오프셋이다.

Event는 Sequence, Kind, Span, Inputs, Result, Currency, TargetScale, Mode, Inexact, Branch를 가진다. Inputs는 문자열 배열이고 TargetScale은 선택적인 uint32다. 이 단계에서는 이벤트 수집기를 구현하지 않는다.

- [x] TestDefaultLimits에 설계서의 모든 숫자를 고정한다.
- [x] TestDiagnosticCodeAndSpan에서 오류를 감싸도 코드와 위치가 보존되는지 확인한다.
- [x] 공통 타입과 Go 모듈을 구현한다.
- [ ] 숫자 표현 탐색용으로 실제 float64 변수, 문자열 직접 덧셈, 공통 스케일 정수 덧셈을 작은 입력에서 비교한다. 이 코드는 제한된 탐색이며 최종 Decimal 엔진이나 일반 성능 결론으로 사용하지 않는다.
- [x] 탐색의 입력·예상 값·관찰·남은 검증을 design-decisions 기록의 첫 항목으로 남긴다. 전체 성능 비교는 Task 14에서 수행한다.
- [x] Run: go test ./internal/config ./internal/diagnostic

**완료:** 숫자·언어·호스트 계층이 공유할 타입에 미정인 이름이 없다.

## Task 2: Decimal 입력과 표시

**Files:** internal/decimal/decimal.go, parse.go, format.go, parse_test.go, format_test.go.

**Consumes:** config.Limits, diagnostic.Code.

**Produces:**

    decimal.Decimal // 외부에서 수정할 수 없는 계수·스케일
    decimal.Parse(text string, limits config.Limits) (Decimal, error)
    (Decimal).String() string
    decimal.FormatAt(x Decimal, digits uint32, limits config.Limits) (string, error)

계수는 비공개 *big.Int다. Decimal의 영값도 0으로 동작한다. 계수 포인터나 변경 가능한 내부 값을 getter로 반환하지 않는다.

- [x] TestParseCanonical에 "12.30"→"12.3", "-0"→"0", "-0.003"→"-0.003"을 적는다.
- [x] TestParseRejectsInvalid에 ".5", "1.", "1e3", "1,000", "NaN", 257바이트 토큰을 적는다.
- [x] TestDecimalBoundaries에서 계수 128자리는 허용, 129자리는 E_NUMERIC_LIMIT인지 확인한다.
- [x] TestFormatExact에서 FormatAt(1.2, 2)="1.20", FormatAt(1.234, 2)=E_INEXACT, digits=37은 E_NUMERIC_LIMIT인지 확인한다.
- [x] big.Int.Set을 사용하는 소유권과 정규화를 구현한다.
- [x] Run: go test ./internal/decimal -run 'TestParse|TestFormat|TestDecimalBoundaries'

## Task 3: 정확한 산술과 비교

**Files:** internal/decimal/arithmetic.go, arithmetic_test.go, oracle_test.go, benchmark_test.go.

**Consumes:** Task 2.

**Produces:**

    decimal.Add(a, b Decimal, limits config.Limits) (Decimal, error)
    decimal.Sub(a, b Decimal, limits config.Limits) (Decimal, error)
    decimal.Mul(a, b Decimal, limits config.Limits) (Decimal, error)
    decimal.Cmp(a, b Decimal) int

- [x] TestArithmeticExact에 0.1+0.2=0.3, 1.2+0.003=1.203, 0.01*0.01=0.0001, 1.20==1.2를 적는다.
- [x] TestOperandsRemainUnchanged에서 b=Add(a,...) 뒤 a가 원래 값인지 확인한다.
- [x] TestArithmeticLimits에서 정규화 후 스케일 36을 허용하고 37은 거부한다. 올림 후 129자리인 덧셈도 거부한다.
- [x] TestRatAgreement에서 고정 시드 20261031로 생성한 10,000쌍의 +,-,*를 big.Rat과 비교한다. 정상 결과와 의도한 범위 오류를 별도 집계한다.
- [x] 서로 다른 스케일을 정렬하고 결과를 정규화한다.
- [x] Run: go test ./internal/decimal -run 'TestArithmetic|TestOperands|TestRatAgreement'

## Task 4: 명시적 반올림과 나눗셈

**Files:** internal/decimal/round.go, division.go, round_test.go, division_test.go, oracle_test.go, experiments/generate_rounding_oracle.py, testdata/rounding_oracle.json.

**Produces:**

    decimal.Mode string
    decimal.ParseMode(text string) (Mode, error)
    decimal.Rounded { Value Decimal; Inexact bool }
    decimal.Round(x Decimal, digits uint32, mode Mode, limits config.Limits) (Rounded, error)
    decimal.Div(a, b Decimal, digits uint32, mode Mode, limits config.Limits) (Rounded, error)

Mode 상수는 HalfEven="HALF_EVEN", HalfUp="HALF_UP", TowardZero="TOWARD_ZERO", Exact="EXACT"다. 잘못된 모드는 E_ROUNDING_MODE다.

- [x] TestSignedTies에서 -2.5를 자리 0으로 줄인 결과가 HALF_EVEN=-2, HALF_UP=-3, TOWARD_ZERO=-2인지 확인한다.
- [x] TestHalfEvenBoundaries에 2.345→2.34, 2.355→2.36, -2.345→-2.34, 9.995→10을 적는다.
- [x] TestDivision에 div(1,8,3,EXACT)=0.125, div(-2,3,2,HALF_EVEN)=-0.67, div(1,3,2,EXACT)=E_INEXACT, 0 나누기 오류를 적는다.
- [x] TestDoubleRounding에서 1.2451→3자리→2자리=1.24와 직접 2자리=1.25를 각각 확인한다.
- [x] generate_rounding_oracle.py에서 Python 표준 decimal의 문자열 입력, precision=512, 명시적 ROUND_HALF_EVEN·ROUND_HALF_UP·ROUND_DOWN 및 Inexact 예외로 독립 기준 파일을 만든다. 0 나누기는 별도 오류로 기록한다.
- [x] 기준 파일은 고정 수작업 사례와 고정 시드 20261031의 round·div 사례를 합쳐 최소 10,000개로 만든다. 부호·절반·자리 올림·목표 스케일 0/36·EXACT 오류를 포함하고 Python 버전·시드·context를 metadata에 기록한다.
- [x] TestRoundingOracleAgreement에서 위 파일의 값·Inexact·오류를 비교한다. Go의 Round·Div 또는 같은 반올림 보조 함수를 정답 생성에 재사용하지 않는다.
- [x] 절댓값의 몫·나머지로 반올림하고 마지막에 부호를 붙인다.
- [x] 나눗셈에서 e=목표스케일+b스케일-a스케일을 구해 분자 또는 분모에 10의 거듭제곱을 적용한다.
- [x] Run: go test ./internal/decimal

Python은 기준 파일을 만드는 개발 도구로만 사용한다. 실행기 의존성에 추가하지 않는다. 기준 파일의 수작업 사례가 일치하는지 먼저 확인하고, 차이가 나면 기준 구현과 런타임을 각각 조사한다.

**의존성:** Task 2~4가 완료되면 숫자 엔진만 독립적으로 검증할 수 있다.

## Task 5: 렉서와 위치

**Files:** internal/token/token.go, internal/lexer/lexer.go, lexer_test.go, fuzz_test.go.

**Produces:**

    token.Kind
    token.Token { Kind Kind; Literal string; Span diagnostic.Span }
    lexer.New(file, source string, limits config.Limits) (*Lexer, error)
    (*Lexer).Next() (token.Token, error)

- [x] TestTokenizeProgram에서 let, fn, return, if, else, 숫자·문자열·비교·괄호·쉼표·세미콜론을 확인한다.
- [x] TestNumericLiteralPreserved에서 0.100의 Literal이 원문 그대로 "0.100"인지 확인한다.
- [x] TestUTF8CommentPosition에서 한글 주석 뒤 오류 줄·열을 확인한다.
- [x] 문자열은 큰따옴표와 이스케이프된 큰따옴표·역슬래시·개행·탭을 지원한다.
- [x] // 주석과 공백을 처리하고 / 자체는 전용 토큰으로 보존한다.
- [x] FuzzLexerNoPanic에 긴 숫자, 닫히지 않은 문자열, 잘못된 UTF-8을 초기 사례로 넣는다. 잘못된 UTF-8은 E_SOURCE_ENCODING이다.
- [x] Run: go test ./internal/lexer

Task 5는 공통 타입 이후 Task 2~4와 병렬로 진행할 수 있다.

## Task 6: AST와 Pratt 파서

**Files:** internal/ast/ast.go, internal/parser/parser.go, parser_test.go.

**Produces:**

    ast.Program
    parser.Parse(file, source string, limits config.Limits) (*ast.Program, error)

노드는 Number, String, Bool, Identifier, Prefix, Infix, Call, Function, Let, If, Return, ExpressionStatement다. 모든 노드에 Span을 둔다.

- [x] TestPrecedence에서 1+2*3과 (1+2)*3의 AST를 구분한다.
- [x] TestParseFunctionsAndIf에서 fn의 매개변수·return·if/else 블록을 확인한다.
- [x] TestSlashRequiresPolicy에서 1/3이 E_DIVISION_POLICY_REQUIRED와 div 안내를 내는지 확인한다.
- [x] TestSyntaxDepth에서 128을 넘는 중첩이 E_PARSE_DEPTH로 끝나는지 확인한다.
- [x] 호출 > 접두 -와 ! > 곱셈 > 덧셈·뺄셈 > 대소 비교 > 동등 비교의 우선순위를 적용한다.
- [x] Run: go test ./internal/parser

## Task 7: 값·변수·분기 평가

**Files:** internal/eval/value.go, environment.go, evaluator.go, builtins.go, evaluator_test.go.

**Consumes:** Task 2~6, trace.Sink.

**Produces:**

    eval.Value interface { Kind() Kind; Inspect() string }
    eval.DecimalValue { Value decimal.Decimal }
    eval.BoolValue, eval.StringValue, eval.UnitValue
    eval.Environment
    eval.NewEnvironment(parent *Environment) *Environment
    (*Environment).Define(name string, value Value) error
    (*Environment).Lookup(name string) (Value, bool)
    eval.Options { Limits config.Limits; Stdout io.Writer; JSONMode bool; Trace trace.Sink }
    eval.New(options Options) *Evaluator
    (*Evaluator).Run(program *ast.Program, env *Environment) (Value, error)

- [x] 테스트 전용 evalSource(t testing.TB, source string) (Value,error)를 정의하고 파싱부터 평가까지 연결한다.
- [x] TestLetArithmetic에서 let x=0.1; let y=x+0.2; y; 의 값이 0.3이고 x는 0.1인지 확인한다.
- [x] TestBranchRequiresBool에서 if(1) {...}는 E_TYPE_MISMATCH인지 확인한다.
- [x] TestNames에서 미정의 이름, 같은 환경의 중복 let, 안쪽 블록의 이름 가리기를 확인한다.
- [x] TestBuiltins에서 div, round, format, print의 인수·타입·모드 오류를 확인한다.
- [x] 각 AST 노드 방문을 세고 MaxEvalSteps 초과를 E_EXECUTION_LIMIT로 처리한다.
- [x] Run: go test ./internal/eval

Trace가 nil이면 이벤트용 문자열을 생성하지 않는다. 핵심판에서는 Sink 연결 지점을 마련하고 실제 기록은 Task 12에서 완성한다.

## Task 8: 함수와 반환

**Files:** internal/eval/function.go, function_test.go, internal/parser/parser_test.go.

**Produces:**

    eval.FunctionValue
    (*Evaluator).Call(fn Value, args []Value, span diagnostic.Span) (Value, error)

- [x] TestSimpleInterest에서 fn(p,r,t){return p*r*t;}를 서로 다른 입력에 재사용한다.
- [x] TestLexicalScope에서 함수가 선언 위치의 바깥 이름을 읽고 지역 이름이 밖으로 새지 않는지 확인한다.
- [x] TestFunctionErrors에서 잘못된 인수 개수, 함수 밖 return, return 없이 끝난 함수를 확인한다.
- [x] TestRecursionLimit에서 호출 깊이 64 초과가 정상 오류인지 확인한다.
- [x] 함수의 환경과 반환 신호를 구현하고 오류를 소스 위치와 연결한다.
- [x] Run: go test ./internal/eval ./internal/parser

## Task 9: 파일 실행과 REPL

**Files:** cmd/fin/main.go, internal/cli/cli.go, cli_test.go, internal/repl/repl.go, repl_test.go, examples/decimal.fin, examples/simple_interest_decimal.fin.

**Produces:**

    cli.Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int
    repl.Run(in io.Reader, out, errOut io.Writer, limits config.Limits) error

- [x] TestFileRun에서 예제 파일을 실행해 예상 출력과 종료 코드를 확인한다.
- [x] TestREPLState에서 앞줄 let 이름을 다음 줄에서 읽고, 오류 뒤 다음 입력을 실행하는지 확인한다.
- [x] REPL은 기본적으로 한 줄 단위다. 여러 줄 함수는 파일 실행을 안내한다. 한 줄 안의 함수 선언은 허용한다.
- [x] TestDiagnostics에 파일 읽기 실패, 1MiB 초과, 잘못된 구문, 평가 제한을 넣는다.
- [x] CLI 성공 0, 소스·평가 오류 1, 사용법·파일 I/O 오류 2를 구현한다.
- [x] Run: go test ./...
- [x] Run: go run ./cmd/fin -file examples/simple_interest_decimal.fin

**v0.1 Gate:** 숫자·문법·오류 테스트, 함수 재사용, 파일·REPL 실행이 확인되어야 다음 단계로 간다.

## Task 10: Money와 명시적 정산

**Files:** internal/money/money.go, money_test.go, internal/eval/money_value.go, money_builtins.go, money_builtins_test.go.

**Produces:**

    money.Money // 비공개 currency와 amount
    money.New(currency string, amount decimal.Decimal) (Money, error)
    (Money).Currency() string
    (Money).Amount() decimal.Decimal
    money.Add(a,b Money, limits config.Limits) (Money,error)
    money.Sub(a,b Money, limits config.Limits) (Money,error)
    money.Cmp(a,b Money) (int,error)
    money.Mul(m Money, factor decimal.Decimal, limits config.Limits) (Money,error)
    money.Div(m Money, divisor decimal.Decimal, digits uint32, mode decimal.Mode, limits config.Limits) (Money,bool,error)
    money.Settle(m Money, digits uint32, mode decimal.Mode, limits config.Limits) (Money,bool,error)
    money.FormatAt(m Money, digits uint32, limits config.Limits) (string,error)
    eval.MoneyValue { Value money.Money }

Money.Div와 Money.Settle의 bool은 Inexact를 나타낸다.

- [x] TestCurrencyRules에서 같은 통화 덧셈·비교, 다른 통화 거부, 지원하지 않는 통화를 확인한다.
- [x] TestMoneyTypeRules에서 Money*Money, Money+Decimal, div(Money,Money,...)를 거부한다.
- [x] TestSettlementTiming에서 USD0.335 세 건의 개별 정산 합=1.02와 합산 후 정산=1.00을 각각 확인한다.
- [x] TestMoneyFormat에서 숫자값 변경 없이 "USD 1.00"을 표시하고, 자리 부족 시 E_INEXACT인지 확인한다.
- [x] money, settle, format_money와 연산자 분기를 연결한다.
- [x] Run: go test ./internal/money ./internal/eval

## Task 11: JSON 계약과 단일 결과

**Files:** internal/contract/contract.go, decode.go, encode.go, contract_test.go, contracts/interest_v1.json, fee_v1.json, sum_v1.json, testdata/inputs/*.json, internal/cli/contract_test.go.

**Produces:**

    contract.Spec
    contract.Load(r io.Reader, limits config.Limits) (Spec,error)
    contract.DecodeInput(r io.Reader, spec Spec, limits config.Limits) ([]eval.Value,error)
    contract.EncodeResult(w io.Writer, spec Spec, result eval.Value) error
    contract.EncodeError(w io.Writer, rule string, err error) error

Spec과 JSON 필드는 설계서 §9를 그대로 구현한다. Spec.Parameters 순서로 main을 호출한다. 숫자 값과 범위는 문자열, scale 메타데이터는 정수다.

- [x] TestContractRoundTrip에서 scale_max=18인 테스트 전용 계약을 사용해 0.100000000000000001과 부호 있는 금액이 float64 변환 없이 보존되는지 확인한다. interest_v1의 이율 입력은 별도로 scale_max=6을 지켜야 한다.
- [x] TestStrictJSON에서 숫자 JSON 값, 중복 키, 미지 필드, 누락 값, 과한 소수 자리, 과한 중첩, 1MiB 초과를 거부한다.
- [x] TestOutputContract에서 타입·통화 불일치, 출력 자리 부족, 수신 필드 범위 초과를 각각 확인한다.
- [x] TestJSONMode에서 stdout 레코드 하나, stderr 진단, print 거부와 종료 0/1/2를 확인한다.
- [x] 입력은 제한된 바이트만 읽고 토큰 검사로 중복 키·중첩을 확인한 뒤 알려진 구조로 디코딩한다.
- [x] Run: go test ./internal/contract ./internal/cli

DecodeInput은 프로그램의 함수를 호출하지 않는다. CLI가 소스를 평가하고 main을 찾아 같은 Evaluator의 Call을 사용해 총 실행 한도를 공유한다.

## Task 12: 실제 평가에서 trace 수집

**Files:** internal/trace/recorder.go, jsonl.go, recorder_test.go, internal/eval/trace_test.go, internal/cli/trace_test.go.

**Produces:**

    trace.NewRecorder(maxEvents int) *Recorder
    (*Recorder).Record(event Event)
    (*Recorder).Events() []Event
    (*Recorder).Truncated() bool
    trace.WriteJSONL(w io.Writer, header Header, recorder *Recorder) error

Header는 프로그램 SHA-256, 계약 버전, 실행기 버전, 입력, 제한 설정을 갖는다. 재현 비교에는 시각·프로세스 ID처럼 달라지는 값을 넣지 않는다.

- [x] TestTraceRecordsExecutedBranch에서 실제 실행한 조건 분기만 기록하는지 확인한다.
- [x] TestTraceRounding에 입력·결과·통화·자리·모드·Inexact·줄번호를 확인한다.
- [x] TestTraceDoesNotChangeValue에서 trace 유무와 10,000개 초과 모두 수치 결과가 같은지 확인한다.
- [x] TestTraceDeterminism에서 같은 소스·입력·설정의 이벤트 순서와 직렬화 결과를 확인한다.
- [x] Task 7의 연결 지점을 채우고 기록을 요청한 CLI의 파일 쓰기 실패는 종료 2로 처리한다.
- [x] trace를 요청한 경로의 상위 디렉터리는 호스트 CLI가 생성하고, 기록이 잘렸는지 파일에 표시한다.
- [x] Run: go test ./internal/trace ./internal/eval ./internal/cli

**v0.2 Gate:** Money 오류 예방, JSON 왕복, 명시적 정산, trace 확인이 끝나야 COBOL 비교를 완료 대상으로 삼는다.

## Task 13: 교육용 규칙과 COBOL 차등 테스트

**Files:** examples/interest_v1.fin, fee_v1.fin, sum_v1.fin, testdata/cobol/interest_v1.cob, fee_v1.cob, sum_v1.cob, tests/compat/compare_test.go, generator_test.go, docs/evaluation/compatibility.md.

**Consumes:** 설계서 §10의 규칙·필드·범위, Task 11 계약.

**Produces:** 실행 환경·소스 해시·총 사례 수·오류 수·일치 수·불일치 원인을 담은 JSONL과 보고서.

- [ ] Linux/WSL에서 Go와 cobc 버전을 확인하고 정확한 컴파일 명령을 기록한다.
- [ ] COBOL의 3개 기준 프로그램에 입력·중간·수신 필드를 고정하고 ON SIZE ERROR를 명시한다.
- [ ] 표준 입력의 문자열을 넘겨 비교하되 os/exec.Command를 직접 사용하고 쉘 문자열 조립을 하지 않는다.
- [ ] 기준 프로그램은 한 번 컴파일하고 테스트 데이터는 두 구현에 동일하게 전달한다.
- [ ] TestCOBOLInterest에서 1000000,0.05,0.5→25000과 999999999.99,1,10→E_LEGACY_RANGE를 확인한다.
- [ ] TestCOBOLFee에서 1000000의 아래·같음·위 경계와 1002000→1001을 확인하고 분기까지 비교한다.
- [ ] TestCOBOLSum에서 0.335 세 건→1.00, 음수 조정, 수신 범위 초과를 확인한다.
- [ ] 각 규칙에 고정 시드 경계·정상·출력 오류 사례를 합쳐 최소 1,000건씩 생성한다.
- [ ] 계약이 거절한 입력은 별도 집계하고 COBOL 실행 일치율의 분모에 섞지 않는다.
- [ ] Run: go test ./tests/compat -run TestCOBOL -count=1 -v

GnuCOBOL이 없으면 테스트는 이유를 출력하고 건너뛸 수 있으나, 이 경우 v0.3 호환성 완료 기준은 충족되지 않는다. 컴파일러별 차이를 발견하면 바뀐 값을 조용히 정답으로 채택하지 말고 프로필과 규칙을 분리해 기록한다.

## Task 14: 성능·사용성·재현 절차

**Files:** experiments/addition_benchmark_test.go, internal/decimal/fuzz_test.go, docs/evaluation/performance.md, usability.md, design-decisions.md, README.md.

- [ ] 문자열 직접 덧셈과 float64 기준 구현을 작은 실험 코드로 둔다. 문자열 방식 전체의 성능이라고 확대하지 않는다.
- [ ] float 비교는 실제 float64 변수·입력을 사용하고 상수 계산 최적화와 구분한다.
- [ ] 입력 해석·숫자 산술·전체 실행을 분리해 짧은 금액·다른 스케일·긴 계수·소액 누적을 측정한다.
- [ ] FuzzDecimalParse(f *testing.F)를 정의해 정상·비정상 숫자와 자릿수 경계를 넣는다. 성공한 입력은 String으로 출력한 뒤 다시 파싱해 같은 값인지 확인한다.
- [ ] Run: go test ./internal/decimal ./experiments -run '^$' -bench . -benchmem -count=5
- [ ] Run: go test ./internal/lexer -run '^$' -fuzz FuzzLexerNoPanic -fuzztime=30s
- [ ] Run: go test ./internal/decimal -run '^$' -fuzz FuzzDecimalParse -fuzztime=30s
- [ ] 발견한 실패 입력을 회귀 테스트에 추가하고 해당 변경이 영향을 주는 검증을 다시 실행한다.
- [ ] 참가자 3~5명에게 동등한 DSL·Python decimal 규칙 수정 과제를 제공한다. 순서와 도움 조건을 기록한다.
- [ ] 완료·시간·실수·도움 요청·설명 정확도를 개인 단위로 남긴다.
- [ ] README에 숫자 의미, 정산 정책, 실행법, 한도, 호환성 범위, 미실행 항목을 적는다.
- [ ] Run: go test ./...
- [ ] Run: go vet ./...
- [ ] Run: go build ./cmd/fin

성능 개선이나 사용자 우위가 관찰되지 않아도 원자료를 그대로 보고한다. 언어 기능 완료와 우위 주장 여부는 별도 판정한다.

## 주차별 배정

| 주차 | 주 작업 | 점검 |
|---|---|---|
| 1 | Task 1~2, 숫자·입출력 예제 확정 | 정상·오류 입력과 제한값 |
| 2 | Task 3~4 | 숫자 엔진 독립 검증 |
| 3 | Task 5~6 | 토큰·AST·연산 우선순위 |
| 4 | Task 7의 산술·변수, 최소 REPL | 수식 실행 여부에 따른 일정 조정 |
| 5 | Task 7의 분기·오류, Task 8 시작 | 타입·스코프 |
| 6 | Task 8 | 함수 재사용·반환·재귀 제한 |
| 7 | Task 9 | 파일 실행·REPL·진단 |
| 8 | 기본판 회귀 검증과 초기 benchmark | v0.1 Gate |
| 9 | Task 10의 Money 연산 | 통화·타입 거부 사례 |
| 10 | Task 10의 settle·표시·예제 | 정산 위치와 반올림 |
| 11 | Task 11 | 계약과 정확한 입출력 |
| 12 | Task 12 | v0.2 Gate |
| 13 | Task 13 기준 COBOL 프로그램 | 규칙·필드·환경 고정 |
| 14 | Task 13 생성 사례와 차이 분석 | 정상·오류·분기 비교 |
| 15 | Task 14 성능·사용 과제 | 원자료와 혼란 지점 |
| 16 | 필요한 수정·재검증·재현 안내 | v0.3 완료 또는 남은 범위 명시 |

## 구현 전 자체 점검

- [x] 기존 폴더와 도구를 확인했다.
- [x] COBOL의 기존 소수 기능과 운영 플랫폼의 책임을 분리했다.
- [x] 숫자·통화·반올림·입출력·오류의 의미를 문서에 정했다.
- [x] 각 모듈의 파일과 공개 인터페이스를 배정했다.
- [x] 정확성·자원 제한·정산 위치·통화·JSON·trace·COBOL 수신 범위를 검증 작업에 연결했다.
- [x] 실제 구현·성능·사용성·COBOL 비교는 이후 실행할 작업으로 남겼다.
- [x] 사용자 구현 요청에 따라 v0.1의 Task 1–9를 구현하고 검증했다.

설계 자체는 독립 검토를 받았으며, 계약과 필드 범위, trace 상한 예외를 구체화했다. 위 체크는 계획 문서의 검토 상태이고 소프트웨어 테스트 통과를 뜻하지 않는다.


## v0.1 실행 결과

v0.1 언어 기능과 숫자 검증을 완료했다. 초기 표현 실험은 문자열에서 유리수로 읽는 방식이며, 문자열 자릿수 직접 산술 비교는 후속으로 남긴다. 실제 결과와 범위는 ../../evaluation/v0.1-verification.md에 기록했다. v0.2/v0.3는 아직 구현하지 않았다.


## v0.2 실행 결과

Money·통화·정산·JSON 계약·실제 계산 기록을 구현했다. 전체 96개 최상위 테스트/퍼즈 함수(하위 사례 포함319개), 독립 숫자 회귀 기준, race/vet/build와 실제 실행 파일 검증을 통과했다. 세부 결과: ../../evaluation/v0.2-verification.md. 사용자 요청에 따라 비공개 GitHub 저장소 Pirite08/finlang에 게시하며, COBOL 비교는 v0.3에 남긴다.
