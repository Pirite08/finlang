# fin v0.1

Go로 만든 학습용 인터프리터입니다. 소수 리터럴을 이진 부동소수점으로 바꾸지 않고 십진 계수와 스케일로 계산합니다. 금융 업무에 필요한 반올림 정책을 직접 지정하는 계산 언어의 기본판입니다.

현재 프로젝트 폴더에는 Windows 실행 파일 build/fin.exe도 생성되어 있습니다. 프로젝트 루트에서 다음처럼 바로 실행할 수 있습니다.

    ./build/fin.exe -file examples/rounding.fin

소수 연산 입력 10,000쌍, 반올림·나눗셈 독립 기준 12,041건과 전체 통합 테스트를 실행했습니다. 구체적인 범위와 원자료는 [v0.1 검증 결과](docs/evaluation/v0.1-verification.md)에 정리했습니다.

## 설치와 실행

Go 1.25 이상에서 저장소 루트를 기준으로 실행합니다.

```powershell
go test ./...
go build -o fin.exe ./cmd/fin
.\fin.exe -version
.\fin.exe -file examples/decimal.fin
.\fin.exe -file examples/simple_interest_decimal.fin
.\fin.exe -file examples/rounding.fin
.\fin.exe
```

빌드 없이 `go run ./cmd/fin -file examples/decimal.fin`으로도 실행할 수 있습니다. `go install ./cmd/fin`은 Go 실행 파일 설치 경로에 `fin`을 설치합니다.

인수 없이 시작하면 한 줄씩 입력하는 REPL이 열립니다. 입력한 이름은 다음 줄에도 남고, 오류가 나면 진단을 출력한 뒤 다음 줄을 읽습니다. 실행 한도는 줄마다 초기화합니다. 입력 스트림을 닫으면 종료합니다. 여러 줄 함수는 UTF-8 파일에 저장하고 `-file`로 실행합니다. 한 줄 안의 함수 선언은 REPL에서도 사용할 수 있습니다.

```text
let amount = 0.1;
amount + 0.2
let twice = fn(x) { return x * 2; };
twice(amount)
```

위 입력의 결과는 차례로 `0.3`, `0.2`입니다. `let`과 `print`는 별도의 결과 값을 출력하지 않습니다. 파일 실행에서는 `print(...)`로 출력할 값을 지정합니다. `-h`는 사용법, `-version`은 `fin v0.1.0`을 출력합니다. 성공은 종료 코드 0, 소스·평가 오류는 1, 사용법·파일 입출력 오류는 2입니다. REPL의 개별 소스·평가 오류는 세션을 종료하지 않습니다.

## 십진수와 반올림

`0.1 + 0.2`는 정확히 `0.3`입니다. 숫자는 정수 또는 `12.34` 형태로 씁니다. 음수는 `-` 접두 연산자로 표현합니다. 지수 표기, `NaN`, `Infinity`, 숫자 구분용 쉼표·밑줄은 지원하지 않습니다. 문자열과 숫자는 자동 변환하지 않습니다.

나눗셈은 `/` 대신 `div(a, b, digits, mode)`를 씁니다. `/`는 `E_DIVISION_POLICY_REQUIRED` 오류를 냅니다. 필요한 소수 자릿수와 반올림 정책을 호출하는 곳에서 지정해야 합니다.

| 모드 | 동작 | `round(-2.5, 0, mode)` |
| --- | --- | --- |
| `HALF_EVEN` | 가장 가까운 값, 정확히 중간이면 남는 마지막 숫자가 짝수인 쪽 | `-2` |
| `HALF_UP` | 가장 가까운 값, 정확히 중간이면 절댓값이 커지는 쪽 | `-3` |
| `TOWARD_ZERO` | 0을 향해 절사 | `-2` |
| `EXACT` | 0이 아닌 자리를 버려야 하면 `E_INEXACT` | 오류 |

```text
div(1, 3, 6, "HALF_EVEN")
round(2.345, 2, "HALF_EVEN")
format(round(9.995, 2, "HALF_EVEN"), 2)
```

결과는 각각 `0.333333`, `2.34`, 문자열 `10.00`입니다. 숫자 값은 불필요한 끝자리 0을 제거해 정규화하므로 `round(9.995, 2, "HALF_EVEN")`의 숫자 표시는 `10`입니다. `format(x, digits)`는 표시용 문자열을 만들며 숨겨서 반올림하지 않습니다. `format(1.234, 2)`는 `E_INEXACT`입니다. 먼저 `round`로 숫자를 조정해야 합니다. 두 번 반올림하면 한 번 반올림한 결과와 달라질 수 있습니다. `examples/rounding.fin`에서 `1.2451`의 사례를 확인할 수 있습니다.

## 구현 범위

v0.1은 Decimal의 `+`, `-`, `*`, 비교, `div`, `round`, `format`, `let`, Bool·String, `if`·`else`, `fn`, `return`, 어휘적 스코프, 재귀, `print`, 파일 실행과 REPL을 제공합니다. `if` 조건에는 Bool만 허용합니다. 같은 환경에서 이름을 다시 선언할 수 없으며, 함수는 `return`으로 값을 반환해야 합니다. 세미콜론은 문장 사이에 쓰고 파일 끝과 닫는 중괄호 직전에는 생략할 수 있습니다.

Money·통화 정책, 정산·배분, JSON 계약 실행, trace 기록, JIT, 자동 형 변환, 문자열 연결, 논리 AND·OR는 이 버전의 제공 범위에 포함하지 않습니다. 설계 문서의 후속 단계와 현재 실행 가능한 기능을 구분해 읽어 주세요.

## 기본 한도

| 항목 | 한도 |
| --- | ---: |
| 정규화한 계수 | 128자리 |
| 소수 스케일 및 자리 인수 | 36 |
| 토큰 | 256바이트 |
| 파일 소스·REPL 한 줄 | 1 MiB |
| 구문 중첩 | 128 |
| 평가 노드 실행 | 100,000 |
| 함수 호출 깊이 | 64 |

한도를 넘으면 오류로 알립니다. 파일은 한도보다 1바이트까지만 읽어 크기를 확인하며, 긴 REPL 줄은 제한된 버퍼로 끝까지 버린 뒤 다음 줄을 처리합니다. 진단에는 가능한 경우 파일명, 행, 열과 오류 코드를 표시합니다.

## 표현 방식 확인 실험

```powershell
go test ./experiments -run TestRepresentationProbe -v
go test ./experiments -bench BenchmarkRepresentationProbe -benchmem
```

작은 실험은 런타임 `float64` 덧셈, 문자열에서 직접 읽은 유리수 덧셈, 같은 스케일을 준비한 정수 계수 덧셈을 비교합니다. 파싱과 메모리 할당 포함 여부가 서로 다르며, 유리수 실험은 실제 Decimal 구현이 아닙니다. 측정값은 실행 환경의 원자료로 읽어야 하며 인터프리터 전체 성능이나 특정 표현 방식의 일반적인 속도 우위를 입증하지 않습니다.
