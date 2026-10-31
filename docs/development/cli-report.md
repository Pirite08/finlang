# v0.1 CLI·REPL 작업 기록

## 구현 범위

- `cmd/fin` 진입점, `cli.Run`, 한 줄 REPL, 예제 3개, 한국어 README.
- 인수 없음: REPL. `-file`: 파일 실행. `-version`: 버전. `-h`: 도움말.
- 소스·평가 오류 1, 사용법·파일 읽기 오류 2, 성공 0.
- 파일 읽기는 `MaxSourceBytes + 1`로 제한. REPL은 긴 줄을 고정 크기 청크로 버린 후 다음 줄을 읽는다.
- REPL 환경은 유지하고 Evaluator는 줄마다 새로 만든다. 식 결과는 Unit이 아닐 때만 표시한다.
- Money, JSON 계약 실행, trace recorder는 추가하지 않았다.

## Red 기록

1. 구현 전에 CLI와 REPL 통합 테스트를 작성했다.
2. `go test ./internal/cli ./internal/repl`은 두 패키지의 `undefined: Run`으로 실패했다. 아직 진입 함수가 없는 예상된 초기 컴파일 실패였다.
3. 진입 함수 구현 후 같은 명령은 `internal/eval` 패키지가 아직 없는 상태에서 실패했다. 다른 작업자가 평가기를 구현 중인 통합 의존성 문제였다. 임시 평가기나 인터페이스를 만들지 않았다.

이 두 실행은 동작 assertion의 실패가 아니라 구현 전의 컴파일 실패다. 평가기가 합쳐진 뒤의 실제 동작 검증은 아래에 별도로 기록한다.

## 표현 방식 실험

`go test ./experiments -run TestRepresentationProbe -v` 통과. 실행 중 변수로 더한 `float64`는 `0.30000000000000004`, 문자열에서 직접 파싱한 유리수는 `0.3`, 같은 스케일 1의 정수 계수 덧셈 결과는 `3`이었다.

`go test ./experiments -bench BenchmarkRepresentationProbe -benchmem -benchtime=100ms` 통과. Windows amd64, Intel i5-11400, Go 벤치마크가 보고한 12 CPU 환경의 단일 짧은 실행 원자료:

| 실험 | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| runtime_float64_add | 0.2869 | 0 | 0 |
| direct_string_rational_parse_and_add | 960.4 | 520 | 22 |
| prepared_common_scale_integer_add | 47.58 | 80 | 2 |

파싱·준비 작업·할당 포함 여부가 서로 다르므로 같은 연산 비용 비교나 속도 우위의 증거로 쓰지 않는다. 유리수 실험은 실제 Decimal 구현도 아니다. 반복 측정이나 전체 인터프리터 성능 검증을 한 결과가 아니다.

## 통합 검증

평가기 구현이 합쳐진 뒤 다음 명령을 실제 실행했다.

- `go test ./internal/cli ./internal/repl -count=1 -v`: 전체 통과. 사용법·버전·종료 코드, 예제 실행, 파일 없음, 1 MiB 초과, 구문 오류, `/` 정책 오류, 실행 한도, REPL 상태 유지, 오류 뒤 계속 실행, 4 KiB보다 긴 줄 버리기, 길이 경계, CRLF, 마지막 줄의 개행 생략, 입력 실패를 확인했다.
- `go test ./... -count=1`: 모든 패키지 통과.
- `go build ./cmd/fin`: 통과. 생성된 `fin.exe`로 `-version`, `-h`, 존재하지 않는 파일의 종료 코드 2를 확인한 뒤 빌드 산출물을 삭제했다.
- `go vet ./...`: 진단 없이 통과.
- `go run ./cmd/fin -file examples/decimal.fin`: `0.3`, `true`, `0.30`.
- `go run ./cmd/fin -file examples/simple_interest_decimal.fin`: `25000`, `0.333333`.
- `go run ./cmd/fin -file examples/rounding.fin`: `-2`, `-3`, `-2`, `-2`, `10.00`, `1.24`, `1.25`.

이 결과는 단위·통합 테스트와 명령행 실행 검증이다. 실제 금융 업무의 정확성 인증이나 성능 우위를 검증한 결과는 아니다.

## 최종 검토 P2 수정: 출력 I/O 오류

`docs/development/final-review.md` 전체를 읽고 출력 실패 분류·전파만 수정했다. 변경 파일은 `internal/cli/cli.go`, `internal/cli/cli_test.go`, `internal/repl/repl.go`, `internal/repl/repl_test.go`, 이 보고서다.

테스트는 `io.Pipe`의 읽기 쪽을 먼저 닫아 실제 표준 라이브러리 writer가 `io.ErrClosedPipe`를 반환하게 했다. 프로세스·goroutine을 만들지 않았고 쓰기 쪽도 닫는다.

Red: 구현 변경 전에 `go test ./internal/cli ./internal/repl -run 'TestOutputFailureIsHostIO|TestREPLPrintOutputFailureStopsSession' -count=1 -v`를 실행해 종료 1을 확인했다. 실제 assertion 실패는 다음과 같았다.

- help: 종료 0, 기대 2, 진단 없음.
- version: 종료 0, 기대 2, 진단 없음.
- 파일 print: 종료 1, 기대 2, `E_OUTPUT_IO: io: read/write on closed pipe`.
- REPL print: 종료 0, 기대 2, `E_OUTPUT_IO: io: read/write on closed pipe`.
- REPL 직접 호출: 반환 오류 nil, 기대 `E_OUTPUT_IO`.

수정은 help/version writer 오류를 확인해 종료 2로 반환하고, 파일 평가의 `E_OUTPUT_IO`를 종료 2로 분류하며, REPL의 같은 코드를 복구 불가능한 호스트 오류로 즉시 반환하도록 했다. REPL 테스트의 다음 줄에는 미정의 이름을 넣고 recoverable 진단 출력이 전혀 없음을 함께 확인해 후속 실행을 막는지 검증했다. 기존 일반 소스·평가 오류 복구와 bare expression 출력·입력 오류 처리는 유지했다.

Green: `go test ./internal/cli ./internal/repl -count=1 -v` 종료 0. 새 회귀 테스트와 기존 파일·REPL·오류 복구 테스트 모두 통과했다. 이어 `go test ./... -count=1` 종료 0으로 전체 패키지가 통과했다.
