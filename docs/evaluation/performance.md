# 표현 방식별 덧셈 측정

2026-10-31. Go 1.25.5, Windows/amd64, Intel Core i5-11400. 아래 수치는 이번 컴퓨터의 미세 측정이며 COBOL과의 속도 비교가 아니다.

## 무엇을 측정했는가

입력 해석, 준비한 값의 덧셈, 소스의 렉싱·파싱·평가를 나눠 측정했다. 세 산술 비교군은 같은 문자열 목록을 미리 읽고, 0에서 시작해 같은 순서로 더한다. 정확한 비교군은 현재 `big.Int` 계수·스케일 구현과 자리별 올림·내림을 직접 처리하는 실험용 문자열 구현이다. `float64`는 정확성 조건이 다른 근삿값 비교군이다.

각 workload 목록 전체가 1 op다. `short_money`는 19.95와 0.05의 2회 덧셈, `mixed_scales`는 1.2·0.003·-0.00004의 3회, `long_coefficient`는 정수부 100자리인 수와 0.01의 2회, `small_accumulation`은 0.01의 100회다. 마지막 행의 ns/op를 단일 덧셈 시간으로 읽으면 안 된다.

산술 측정에는 결과 정규화와 숫자 한도 검사가 포함되며 입력 해석·표시는 제외된다. `BenchmarkNumericParsing`은 해석만, `BenchmarkInterpreterExecution`은 소스의 렉싱·파싱·새 환경에서의 평가를 포함한다. 후자는 프로세스 시작·파일 I/O·JSON·trace를 포함하지 않는다.

## 이번 측정의 중앙값

단위 ns/op, 각각 5회 중앙값이다. 전체 원자료의 B/op·allocs/op·최솟값·최댓값도 함께 보관했다.

| workload | float64 | 현재 스케일드 정수 | 실험용 문자열 직접 덧셈 |
|---|---:|---:|---:|
| short_money, 2회 | 0.773 | 796.3 | 109.1 |
| mixed_scales, 3회 | 0.967 | 1,292 | 162.0 |
| long_coefficient, 2회 | 0.784 | 1,804 | 668.8 |
| small_accumulation, 100회 | 51.68 | 32,432 | 4,171 |

이번 덧셈 비교에서는 실험용 문자열 구현이 현재 스케일드 정수 구현보다 빨랐다. 따라서 스케일드 정수로 바꿨기 때문에 속도가 개선됐다는 주장은 하지 않는다. 문자열 방식 전체, 곱셈·나눗셈·정산까지 포함한 엔진 전체의 우열을 이 덧셈 실험으로 판단할 수도 없다. 현재 엔진을 채택한 근거는 정확한 값 표현과 정산·오류 정책의 일관성이다. 최적화는 별도 프로파일링 후 평가할 작업이다.

float64의 짧은 작업은 컴파일러 최적화와 타이머의 영향을 크게 받는 미세 측정이다. 이 숫자는 같은 정확성을 제공하는 대안의 처리량을 뜻하지 않는다. 또 이 컴퓨터의 다른 프로그램 부하를 통제하지 않았고 200ms 측정 구간의 반복 편차가 있으므로 작은 차이를 성능 개선으로 해석하지 않는다.

## 정확도는 별도로 확인했다

실험용 문자열 덧셈은 수작업 앵커와 독립 `math/big.Rat` 10,000쌍으로 확인했다. 현재 Decimal·실험 문자열·인터프리터 전체 실행은 네 workload의 정확한 기준값과 일치했다. `float64` 값은 문자열 상수의 컴파일 시 계산이 아니라 실제 입력을 `strconv.ParseFloat`로 읽은 뒤 누적한 값이다.

| workload | 정확한 결과 | float64 최종 표시 |
|---|---|---|
| short_money | 20 | 20 |
| mixed_scales | 1.20296 | 1.2029599999999998 |
| long_coefficient | 1 뒤에 0이 100개 | 1e+100, 저장값은 정확한 정수와 다름 |
| small_accumulation | 1 | 1.0000000000000007 |

정확한 유리수 차이는 [실험 기록](../development/v03-experiments-report.md)에 있다. 같은 화면 표시가 나와도 저장값이 정확하다는 뜻은 아니다.

## 원자료와 재현

- [측정 원문 140개 결과](v0.3-benchmarks.txt): 28개 측정 × 5회.
- [원자료와 중앙값 JSON](v0.3-benchmark-summary.json): ns/op, B/op, allocs/op, 범위.
- 코드: `experiments/addition_benchmark_test.go`, `addition_correctness_test.go`.

실제로 실행한 명령:

```sh
go test ./experiments -run '^$' -bench 'Benchmark(NumericParsing|AdditionRepresentation|InterpreterExecution)$' -benchmem -benchtime=200ms -count=5
go test ./experiments -run TestAdditionWorkloads -count=1 -v
```

첫 명령은 다른 benchmark·지속 fuzz와 겹치지 않게 실행했다. 실행 시간을 제한하기 위해 원래 계획의 기본 측정 시간 대신 `-benchtime=200ms`를 명시했다. 더 긴 측정은 이 옵션을 생략해 재현할 수 있다. 기존 `BenchmarkRepresentationProbe`는 비교군마다 파싱 포함 범위가 다르므로 위 표에 합치지 않았다.
