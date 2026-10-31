# v0.3 표현 실험과 사용 과제 준비 기록

2026-10-31. 이 기록의 범위는 Task 14의 덧셈 실험 코드, Decimal 파싱 fuzz 표적의 분리·확장, 사용 과제 계획이다. 런타임 숫자 엔진·CLI·COBOL 비교 구현은 변경하지 않았다. 실제 참가자 평가는 미실행이다.

## 시험하는 범위

`experiments/addition_benchmark_test.go`의 문자열 표현은 부호, ASCII 계수 문자열, 스케일을 보관한다. 입력을 검증하고 스케일을 맞춘 뒤 각 자리의 올림·빌림으로 부호 있는 덧셈을 수행한다. 산술 내부에서 `big.Int`, `big.Rat`, `float64`, 런타임 Decimal을 호출하지 않는다. 반면 정확한 스케일드 인터저 비교군은 실제 런타임 `decimal.Parse`와 `decimal.Add`다. 두 정확한 비교군 모두 정규화와 기본 계수 128자리·스케일 36 제한을 확인한다.

이 구현은 덧셈 전용 실험이며 문자열 방식의 곱셈·나눗셈·전체 숫자 엔진을 대표하지 않는다. `float64` 비교군은 원래 입력 문자열을 실행 중 `strconv.ParseFloat`로 읽은 변수를 사용한다. 컴파일 시 십진 상수 계산을 float64 실행 시간으로 오해하지 않는다. 부동소수점의 정확도 조건은 두 정확한 비교군과 다르다.

## 확인한 RED와 GREEN

실제 자릿수 구현 전에 앵커·잘못된 입력·한도·독립 유리수 대조 테스트를 먼저 적었다. 컴파일 가능한 임시 표적은 숫자 `0`과 성공을 반환했다. 다음 명령은 종료 코드 1로 실패했다.

```powershell
go test ./experiments -run 'TestDigitStringAdditionAnchors|TestDigitStringValidationAndLimits|TestDigitStringAdditionRatAgreement' -count=1
```

관찰한 실패에는 `0.1 + 0.2 = "0"; want 0.3`, 잘못된 `.5` 입력이 오류 없이 통과함, 128자리 9에 1을 더해도 한도 오류가 없음이 포함됐다. 고정 시드의 첫 독립 비교도 `20802252.1 + 704739018 = 0; want 7255412701/10`으로 실패했다. 이는 기능이 없는 임시 표적에서 확인한 RED이며 기존 Decimal 엔진에서 발견한 결함이 아니다. 최종 파일에는 이 임시 표적이 없다.

직접 덧셈을 구현한 뒤 같은 테스트가 통과했다. `TestDigitStringAdditionRatAgreement`는 seed=20261031, 정수 계수 -1,000,000,000~1,000,000,000, 스케일 0~6의 10,000쌍을 생성한다. 독립 `math/big.Rat`는 정답 비교에만 사용한다. 긴 계수·혼합 스케일·양수와 음수·상쇄·자리 올림은 별도 수작업 앵커로 확인한다. 시험 코드는 런타임 불변성 계약과 같은 방식으로 피연산자 값 보존도 확인한다.

```powershell
go test ./experiments -run 'TestDigitString|TestAdditionWorkloads' -count=1 -v
go test ./internal/decimal ./experiments -count=1
```

두 명령 모두 이 작업에서 통과했다. 마지막 명령의 두 패키지는 각각 `ok`였으며 지속 fuzz나 benchmark를 실행하는 명령은 아니다. 전체 저장소 검증 결과는 최종 통합 기록에서 별도로 확인해야 한다.

## 같은 입력과 연산 단위

모든 산술 비교군은 0에서 시작해 목록의 순서대로 각 입력을 한 번씩 더한다. 다음 표의 한 행 전체가 benchmark의 1 op다. 따라서 100회 누적과 2회 덧셈의 ns/op를 단일 덧셈 비용인 것처럼 직접 비교하지 않는다. 산술·전체 실행에는 `adds/op`, 입력 해석에는 `values/op`를 함께 출력한다.

| workload | 입력 | 목록당 덧셈 수 | 정확한 결과 |
|---|---|---:|---|
| short_money | 19.95, 0.05 | 2 | 20 |
| mixed_scales | 1.2, 0.003, -0.00004 | 3 | 1.20296 |
| long_coefficient | 정수부가 9로 100자리인 수의 소수부 .99, 0.01 | 2 | 1 뒤에 0이 100개인 정수 |
| small_accumulation | 0.01을 100개 | 100 | 1 |

`TestAdditionWorkloads`에서 두 정확한 표현과 전체 인터프리터 결과를 수작업 앵커·독립 유리수 합계와 대조했다. 같은 입력 순서에서 관찰한 float64 최종 값과 정확한 합계의 차이는 다음과 같다. 출력이 짧게 `1e+100`으로 보이는 것과 실제 저장값이 정확히 그 정수인 것은 구분한다.

| workload | float64 최종 표시 | float64 저장값 - 정확한 합계 |
|---|---|---|
| short_money | 20 | 0 |
| mixed_scales | 1.2029599999999998 | -2713 / 14073748835532800000 |
| long_coefficient | 1e+100 | 159028911097599180468360808563945281389781327557747838772170381060813469985856815104 |
| small_accumulation | 1.0000000000000007 | 3 / 4503599627370496 |

한 사례의 합계 오차가 0이라는 관찰을 float64의 십진 정확성으로 일반화하지 않는다. 긴 계수에서는 입력 해석부터 표현 손실이 있으며, 여기에 덧셈을 수행한 최종 차이를 보고한 것이다.

## 측정 경계와 재현 명령

- `BenchmarkNumericParsing`: 같은 원문 목록의 입력 해석만 측정한다. 덧셈은 없다.
- `BenchmarkAdditionRepresentation`: 미리 읽은 같은 값 목록의 덧셈·결과 정규화·한도 확인을 측정한다. 입력 해석과 출력 문자열 생성은 제외한다. float64에는 같은 정확성 보장이 없다.
- `BenchmarkInterpreterExecution`: 같은 덧셈 목록을 `let` 문장으로 만든 소스의 렉싱·파싱·새 평가기와 환경 생성·정확한 평가를 측정한다. 소스 문자열 생성, 프로세스 시작, CLI 파일 I/O, 결과 표시, JSON·trace는 제외한다. 따라서 CLI 전체의 비용으로 부르지 않는다.

아래 명령은 새 비교군만 5회 측정한다. 다른 benchmark나 fuzz를 동시에 실행하지 않고 CPU·Go 버전·OS·부하 조건과 원문 출력을 함께 보관한다. ns/op, B/op, allocs/op의 5개 원자료를 남긴 뒤 동일 workload·동일 경계 안에서 해석한다.

```powershell
go test ./experiments -run '^$' -bench 'Benchmark(NumericParsing|AdditionRepresentation|InterpreterExecution)$' -benchmem -count=5
```

Task 14의 전체 benchmark 명령은 다음과 같다.

```powershell
go test ./internal/decimal ./experiments -run '^$' -bench . -benchmem -count=5
```

기존 `BenchmarkRepresentationProbe`에는 파싱을 포함한 유리수와 준비된 정수처럼 서로 다른 경계가 있다. 해당 옛 탐색 결과를 새 세 표현의 공정한 비교에 합치지 않는다. 이 작업자는 동시 측정 간섭을 피하려고 benchmark를 실행하지 않았으며, 통합 실행 담당자가 남긴 최종 수치·원자료는 [performance.md](../evaluation/performance.md)에서 관리한다. 이 문서는 측정하지 않은 성능 수치나 개선 결론을 제시하지 않는다.

## 파싱 fuzz와 사용 과제의 상태

기존 `FuzzDecimalParse`를 `parse_test.go`에서 `fuzz_test.go`로 옮겼다. 이름과 런타임 입력 의미는 유지했다. 정상·비정상 문법, 잘못된 UTF-8, 계수 127/128/129자리, 스케일 35/36/37, 원문 255/256/257바이트, 정규화 후 한도 안에 들어오는 긴 0 사례를 초기 입력으로 둔다. 성공한 입력은 `String()`으로 출력해 다시 파싱하고 수치 비교와 정규형 안정성을 확인한다. 원문 제한 검사를 가리는 별도 길이 skip은 두지 않는다.

이 작업에서는 위 일반 테스트 명령으로 seed 사례만 실행했다. 아래 지속 fuzz는 통합 실행 담당자가 다른 측정과 겹치지 않게 실행한다. 발견한 실패가 있으면 실제 입력·오류·수정·회귀 검증을 최종 기록에 추가해야 한다. 실행 없이 “새 실패 없음”으로 보고하지 않는다.

```powershell
go test ./internal/decimal -run '^$' -fuzz FuzzDecimalParse -fuzztime=30s
go test ./internal/lexer -run '^$' -fuzz FuzzLexerNoPanic -fuzztime=30s
```

[usability.md](../evaluation/usability.md)는 3~5명에게 제공할 동등한 DSL·Python decimal 과제, 순서 교대, 도움 조건, 완료·시간·실수·요청·설명 판정과 빈 기록지를 담는다. **참가자 데이터는 없다.** 준비한 평가 절차를 실제 접근성 개선이나 v0.3 사용성 검증 완료로 표현하지 않는다.
