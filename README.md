# finlang

**Exact decimals. Explicit settlement. Inspectable calculations.**

`v0.2` · `Go 1.25+` · `표준 라이브러리만 사용`

금액을 언제, 몇 자리에서, 어떤 규칙으로 정산할지 코드에 직접 쓰는 작은 인터프리터입니다. 십진수는 이진 부동소수점으로 바꾸지 않고 계산하며, `Money`는 통화를 함께 보존합니다. JSON 계약으로 입력과 출력을 검사하고, 실제 계산 과정은 JSONL trace로 확인할 수 있습니다.

학습·검증을 위한 프로젝트입니다. 현재 버전은 금융 시스템의 운영 적합성이나 COBOL 호환성을 보증하지 않습니다.

[빠른 시작](#빠른-시작) · [정산 시점](#정산-시점을-코드에-남기기) · [JSON 계약](#json-계약으로-규칙-실행하기) · [계산 기록](#실제로-실행한-계산-기록하기) · [검증](#검증-재현하기)

## 빠른 시작

Go 1.25 이상을 설치하고 저장소 루트에서 실행합니다. 실행 파일 이름은 `fin`입니다.

처음 받는 경우:

```sh
git clone https://github.com/Pirite08/finlang.git
cd finlang
```

```sh
go test ./...
go run ./cmd/fin -version
go run ./cmd/fin -file examples/decimal.fin
go run ./cmd/fin -file examples/interest_v1.fin -contract contracts/interest_v1.json -input testdata/inputs/interest.json -json -trace build/interest.trace.jsonl
```

마지막 명령은 원금 `1000000 KRW`, 연이율 `0.05`, 기간 `0.5`로 단리 이자를 계산합니다. 표준 출력에는 다음 JSON 한 건을 기록합니다.

```json
{"schema_version":"1","rule":"interest_v1","status":"ok","result":{"currency":"KRW","amount":"25000"}}
```

빌드해 사용하려면:

```sh
go build -o build/fin.exe ./cmd/fin
```

Windows PowerShell에서는 `.\build\fin.exe`, macOS·Linux에서는 `go build -o build/fin ./cmd/fin` 후 `./build/fin`으로 실행합니다. `-h`는 사용법, `-version`은 `fin v0.2.0`을 출력합니다.

인수 없이 `go run ./cmd/fin`을 실행하면 한 줄 REPL이 열립니다. 선언한 이름은 다음 줄에도 남습니다. 여러 줄 함수는 UTF-8 파일에 저장한 뒤 `-file`로 실행합니다. 파일에서 값을 출력하려면 `print(...)`를 사용합니다.

## 정산 시점을 코드에 남기기

`0.335 USD` 세 건을 합산한 뒤 정산하는 경우와, 한 건씩 정산한 뒤 더하는 경우는 결과가 다릅니다.

```text
let item = money("USD", 0.335);

let total = settle(item + item + item, 2, "HALF_EVEN");
print(format_money(total, 2));

let settled_item = settle(item, 2, "HALF_EVEN");
print(format_money(settled_item + settled_item + settled_item, 2));
```

```text
USD 1.00
USD 1.02
```

`money("USD", 0.335)`는 생성할 때 센트 단위로 반올림하지 않습니다. 덧셈과 곱셈도 허용 범위에서 정확한 값을 유지하며, `settle(value, digits, mode)`를 호출한 위치에서 정산합니다. KRW라는 이유로 자동으로 정수 금액을 만들지도 않습니다.

| 연산 | 규칙 |
| --- | --- |
| `Money + Money`, `Money - Money`, 비교 | 같은 통화끼리만 허용 |
| `Money * Decimal`, `Decimal * Money` | 금액에 비율을 곱함 |
| `div(Money, Decimal, digits, mode)` | 나눗셈의 자릿수와 정책을 명시 |
| `settle(Money, digits, mode)` | 지정한 자릿수로 정산 |
| `format_money(Money, digits)` | 통화와 고정 소수 자릿수를 가진 문자열 생성 |

지원 통화는 `KRW`, `USD`입니다. 서로 다른 통화의 덧셈·비교는 `E_CURRENCY_MISMATCH`, 다른 통화 생성은 `E_UNSUPPORTED_CURRENCY`입니다. 환산은 제공하지 않습니다. `Money + Decimal`, `Money * Money`도 허용하지 않습니다.

### 숫자와 표시 문자열

숫자는 불필요한 소수부 끝자리 0을 제거합니다. `money("USD", 1.00)`의 숫자 표시는 `USD 1`이며, `format_money(..., 2)`로 표시 문자열 `USD 1.00`을 만듭니다. 끝자리 0은 표시 형식이며 값에 저장된 정산 이력이 아닙니다.

`format_money(money("USD", 1.234), 2)`는 값을 몰래 반올림하지 않고 `E_INEXACT`를 냅니다. 먼저 `settle`로 정책을 적용해야 합니다. Decimal의 `format(x, digits)`도 같은 원칙을 따릅니다.

## 십진수와 반올림 정책

`0.1 + 0.2`는 정확히 `0.3`입니다. 정수와 `12.34` 형태의 소수 리터럴을 지원하고, 음수는 `-` 접두 연산자로 표현합니다. 지수 표기, `NaN`, `Infinity`, 숫자 구분용 쉼표·밑줄과 문자열의 자동 숫자 변환은 지원하지 않습니다.

나눗셈은 `div(a, b, digits, mode)`를 사용합니다. `/`는 정책을 생략할 수 없도록 `E_DIVISION_POLICY_REQUIRED`를 냅니다. Decimal에는 `round`, Money에는 `settle`을 사용합니다.

| 모드 | 처리 규칙 | `round(-2.5, 0, mode)` |
| --- | --- | --- |
| `HALF_EVEN` | 가장 가까운 값, 정확히 중간이면 마지막 숫자가 짝수인 쪽 | `-2` |
| `HALF_UP` | 가장 가까운 값, 정확히 중간이면 절댓값이 커지는 쪽 | `-3` |
| `TOWARD_ZERO` | 0을 향해 절사 | `-2` |
| `EXACT` | 0이 아닌 자리를 버려야 하면 `E_INEXACT`로 거부 | 오류 |

```text
print(div(1, 3, 6, "HALF_EVEN"));
print(round(2.345, 2, "HALF_EVEN"));
print(format(round(9.995, 2, "HALF_EVEN"), 2));
```

결과는 `0.333333`, `2.34`, `10.00`입니다. `EXACT`는 더 정밀한 근삿값을 만드는 모드가 아니라, 지정한 자릿수에서 값 손실을 허용하지 않는 모드입니다.

## JSON 계약으로 규칙 실행하기

소스는 계산식을, 계약은 진입 함수와 입출력 조건을 정의합니다. 계약은 매개변수의 순서·타입·통화·범위·최대 소수 자릿수와 출력의 통화·범위·소수 자릿수를 검사합니다.

[이자 규칙](examples/interest_v1.fin):

```text
let main = fn(principal, annual_rate, years) {
    return settle(principal * annual_rate * years, 0, "HALF_UP");
};
```

[입력](testdata/inputs/interest.json):

```json
{
  "schema_version": "1",
  "rule": "interest_v1",
  "inputs": {
    "principal": {"currency": "KRW", "amount": "1000000"},
    "annual_rate": "0.05",
    "years": "0.5"
  }
}
```

금액·비율·기간과 계약의 `min`·`max`는 모두 **JSON 문자열**입니다. JSON 숫자로 읽으면서 정밀도를 잃지 않도록 `"0.05"`처럼 전달합니다. 계약 메타데이터의 `scale`, `scale_max`는 정수입니다. 입력의 `scale_max`는 끝자리 0을 제거하기 전 원문 소수 자릿수에도 적용됩니다.

출력 `amount`는 계약의 `scale`에 맞춘 문자열입니다. 출력 인코더가 정산을 대신하지 않습니다. 예를 들어 출력 `scale: 2`에 `1.005`를 반환하면 `E_OUTPUT_SCALE`로 거부합니다. 필요한 정산은 소스에서 명시해야 합니다. JSON 결과 모드에서 `print`는 `E_OUTPUT_SIDE_EFFECT`로 거부됩니다.

| 규칙 | 소스 / 계약 / 입력 | 포함된 입력의 결과 |
| --- | --- | --- |
| `interest_v1` | [소스](examples/interest_v1.fin) · [계약](contracts/interest_v1.json) · [입력](testdata/inputs/interest.json) | `KRW 25000` |
| `sum_v1` | [소스](examples/sum_v1.fin) · [계약](contracts/sum_v1.json) · [입력](testdata/inputs/sum.json) | `USD 1.00` |
| `fee_v1` | [소스](examples/fee_v1.fin) · [계약](contracts/fee_v1.json) · [입력](testdata/inputs/fee.json) | `KRW 1500` |

다른 규칙을 실행하려면 빠른 시작 명령의 소스·계약·입력 파일을 해당 행의 파일로 바꿉니다. `-file`, `-contract`, `-input`, `-json`을 함께 사용하고, trace가 필요할 때 `-trace`를 추가합니다.

## 실제로 실행한 계산 기록하기

`-trace build/interest.trace.jsonl`은 실제 평가 중 발생한 연산·함수 호출·분기·정산 등을 한 줄에 하나의 JSON 레코드로 기록합니다. 소스를 분석해 예상 경로를 만들어 낸 기록이 아닙니다.

| 레코드 | 내용 |
| --- | --- |
| `header` | 소스 SHA-256, 계약·실행기 버전, 입력, 실행 한도 |
| `event` | 순번, 소스 위치, 입력·결과, 해당되는 경우 통화·목표 자릿수·모드·분기 |
| `summary` | 기록한 이벤트 수와 `trace_truncated` |

`inexact: true`는 해당 정산·나눗셈에서 0이 아닌 자리를 버렸다는 뜻입니다. 기본 기록 한도는 **10,000개 이벤트**입니다. 한도를 넘으면 추가 이벤트를 버리고 `trace_truncated: true`를 남기며, 계산 자체는 계속합니다. 따라서 잘린 trace는 전체 실행 기록으로 해석하면 안 됩니다.

같은 소스·입력·계약·실행 설정에는 같은 기록을 생성하도록 설계했습니다. trace는 입력과 중간 금액을 포함하므로 공유할 때 그 내용도 함께 확인해야 합니다.

## 언어 범위와 실행 한도

`let`, Decimal·Money·Bool·String, 비교, `if`·`else`, `fn`·`return`, 어휘적 스코프·클로저·재귀를 지원합니다. `if` 조건은 Bool만 허용합니다. 같은 환경에서 이름을 다시 선언할 수 없고 함수는 `return`으로 값을 반환해야 합니다. 세미콜론은 문장 사이에 쓰며 파일 끝과 닫는 중괄호 직전에는 생략할 수 있습니다.

| 항목 | 기본 한도 |
| --- | ---: |
| 정규화한 십진 계수 | 128자리 |
| 소수 스케일 및 자리 인수 | 36 |
| 토큰 | 256바이트 |
| 파일 소스·REPL 한 줄 | 1 MiB |
| 구문 중첩 | 128 |
| 평가 노드 실행 | 100,000 |
| 함수 호출 깊이 | 64 |
| trace 이벤트 | 10,000 |

한도를 넘으면 오류로 알립니다. 진단은 가능한 경우 파일명·행·열·오류 코드를 포함합니다. CLI 종료 코드는 성공 `0`, 소스·계산·계약 오류 `1`, 사용법·파일 입출력 오류 `2`입니다. REPL은 개별 계산 오류 후 다음 줄을 읽으며 실행 한도는 줄마다 초기화합니다.

자동 형 변환, 문자열 연결, 논리 AND·OR, 환율 변환, 배분 연산, JIT, COBOL 실행·호환 계층은 현재 제공하지 않습니다.

## 코드 구조

```text
UTF-8 source → lexer → parser → AST → evaluator → result
                                       ↑          ↓
                               JSON contract   JSON / trace
```

| 경로 | 역할 |
| --- | --- |
| [`cmd/fin`](cmd/fin), [`internal/cli`](internal/cli) | 실행 진입점과 CLI |
| [`internal/decimal`](internal/decimal) | 십진 표현·정확한 연산·반올림·표시 |
| [`internal/money`](internal/money) | 통화 검사와 명시적 정산 |
| [`internal/lexer`](internal/lexer), [`internal/parser`](internal/parser), [`internal/ast`](internal/ast) | 토큰·문법·구문 트리 |
| [`internal/eval`](internal/eval), [`internal/repl`](internal/repl) | 평가·내장 함수·대화형 실행 |
| [`internal/contract`](internal/contract) | JSON 계약과 입력·출력 검증 |
| [`internal/trace`](internal/trace) | 제한된 결정적 이벤트 기록과 JSONL 직렬화 |
| [`internal/config`](internal/config), [`internal/diagnostic`](internal/diagnostic) | 실행 한도와 위치 기반 오류 |
| [`examples`](examples), [`contracts`](contracts), [`testdata`](testdata) | 실행 예제·계약·검증 입력 |

## 검증 재현하기

```sh
go test -count=1 ./...
go vet ./...
go test ./internal/eval -run TestMoneySettlementTiming -v
go test ./internal/contract ./internal/trace -v
```

v0.1 기준 검증에서는 소수 입력 10,000쌍을 `math/big.Rat`과 비교하고, 반올림·나눗셈 12,041건을 독립 기준과 대조했습니다. 이는 **v0.1 숫자 연산 검증**이며 v0.2 전체의 테스트 건수를 뜻하지 않습니다.

v0.2에서도 이 숫자 회귀 검증을 다시 통과했습니다. 최종 통합 검증은 최상위 테스트·퍼즈 함수 **96개**, 하위 사례 포함 **319개**가 통과했으며, 빌드한 실행 파일로 예제와 오류 경로 **18개**를 확인했습니다.

- [v0.1 검증 범위와 결과](docs/evaluation/v0.1-verification.md)
- [v0.2 검증 범위와 결과](docs/evaluation/v0.2-verification.md)
- [설계 선택과 제약](docs/evaluation/design-decisions.md)

표현 방식 실험은 `go test ./experiments -run TestRepresentationProbe -v`와 `go test ./experiments -bench BenchmarkRepresentationProbe -benchmem`으로 재현할 수 있습니다. 파싱·메모리 할당 포함 조건이 달라, 이 측정만으로 인터프리터 전체나 특정 숫자 표현의 속도 우위를 판단하지 않습니다.

## 다음 단계

| 버전 | 범위 |
| --- | --- |
| v0.1 | Decimal과 언어·파일 실행·REPL 기반 |
| v0.2 | Money, 명시적 정산, JSON 계약 실행, 실제 실행 trace |
| v0.3 계획 | 제한된 규칙 범위에서 COBOL과 결과 비교 및 경계 사례 검증 |

v0.3은 후속 계획입니다. 현재 구현을 COBOL 대체품이나 호환 실행기로 설명하지 않습니다.
